package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path"
	"strings"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
	"github.com/faiface/pixel/pixelgl"
	"github.com/faiface/pixel/text"

	E "github.com/JoshPattman/goevo"
)

//go:embed sprites
var spritesFS embed.FS

var (
	BoatSprite   *pixel.Sprite
	SailSprite   *pixel.Sprite
	RudderSprite *pixel.Sprite
)

// Create the environment that the boats will be trained in
// In the future, it could be cool to add some noise to these params
func BasicEnv(track []pixel.Vec, boats ...*Boat) *Env {
	return &Env{
		WindVec:             pixel.V(2, 0),
		Boats:               boats,
		WindForceMultiplier: 1,
		PointHitDist:        1,
		Track:               track,
		CurrentPoints:       make([]int, len(boats)),
		NumWaypointsReached: make([]int, len(boats)),
	}
}

// Command line inputs
var (
	isTraining    bool
	genoFileNames string

	targetPopSize  int
	maxGenerations int
	simsPerGen     int
	simSteps       int
	targetSpecies  int
	activations    []E.Activation
)

func main() {
	// Parse CLI
	flag.BoolVar(&isTraining, "train", false, "if specified, will train a new network")
	flag.StringVar(&genoFileNames, "filename", "models/model.json", "the filename of the model to save/load, if loading for playback you can load multiple by using colons file1.json:file2.json:file3.json")

	flag.IntVar(&targetPopSize, "pop-size", 250, "(training only) target population size")
	flag.IntVar(&maxGenerations, "max-generations", 100, "(training only) number of generations to train for")
	flag.IntVar(&simsPerGen, "sims-per-gen", 10, "(training only) number of tracks for each agent to race on per generation")
	flag.IntVar(&simSteps, "sim-steps", 60*20, "(training only) number of steps each training sim should use, 60 steps is 1 second")
	flag.IntVar(&targetSpecies, "target-species", 15, "(training only) target number of species during training")
	var rawAcList string
	flag.StringVar(&rawAcList, "activations", "reln:sigmoid", "(training only) colon seperated list of activations that can be used during training, one of: linear relu tanh reln sigmoid relumax step")

	flag.Parse()
	activations = readActivationList(rawAcList)

	fmt.Println("Parsed CLI")

	// Run the program
	if isTraining {
		fmt.Println("Running training loop")
		train()
	} else {
		fmt.Println("Loading sprites")
		loadSprites()
		fmt.Println("Running visualisation")
		pixelgl.Run(race)
	}
}

// Run a race with a window to show the positions of the boats
func race() {
	// Setup window
	win := must(pixelgl.NewWindow(pixelgl.WindowConfig{
		Title:  "Very physics-based boat sim",
		Bounds: pixel.R(0, 0, 800, 800),
		VSync:  true,
	}))

	// Load AIs from files
	boatNames := []string{"user"}
	fileNames := strings.Split(genoFileNames, ":")
	genos := make([]*E.Genotype, 0)
	phenos := make([]*E.Phenotype, 0)
	for _, n := range fileNames {
		geno := E.NewGenotypeEmpty()
		f := must(os.Open(n))
		json.NewDecoder(f).Decode(&geno)
		f.Close()
		pheno := E.NewPhenotype(geno)
		genos = append(genos, geno)
		phenos = append(phenos, pheno)
		boatNames = append(boatNames, path.Base(n))
	}

	// Create an image of the first specified AIs brain. Make it a sprite
	vis := E.NewGenotypeVisualiser()
	vis.ImgSizeX = 256
	vis.ImgSizeY = 256
	vis.NeuronSize = 5
	genoImg := vis.DrawImage(genos[0])
	genoPic := pixel.PictureDataFromImage(genoImg)
	genoSprite := pixel.NewSprite(genoPic, genoPic.Bounds())

	// Create test env with a user boat and also a boat for each AI. The user boat is first
	boats := repeated(NewDefaultBoat, len(genos)+1)
	env := BasicEnv(NewTrack(12, 10), boats...)
	userBoat := env.Boats[0]

	// Specify the colors for all boats
	colors := []pixel.RGBA{
		pixel.RGB(1, 1, 1), // player

		pixel.RGB(1, 0, 0),
		pixel.RGB(0.5, 0, 0),

		pixel.RGB(0, 1, 0),
		pixel.RGB(0, 0.5, 0),

		pixel.RGB(0, 0, 1),
		pixel.RGB(0, 0, 0.5),

		pixel.RGB(1, 0, 1),
		pixel.RGB(0.5, 0, 0.5),

		pixel.RGB(0, 1, 1),
		pixel.RGB(0, 0.5, 0.5),

		pixel.RGB(1, 1, 0),
		pixel.RGB(0.5, 0.5, 0),
	}

	// Setup the transforming from world space to screen space
	var zoom float64 = 35.0
	var offset pixel.Vec = pixel.V(0, 0)
	worldToScreenSpace := func(mat pixel.Matrix) pixel.Matrix {
		return mat.Moved(offset).Scaled(pixel.ZV, zoom).Moved(win.Bounds().Center())
	}

	// Create imdraws
	windImd := imdraw.New(nil)
	trackImd := imdraw.New(nil)

	// Create the text we will use to draw the names of the boats
	nameText := text.New(pixel.V(0, 0), text.Atlas7x13)

	// Update window
	for !win.Closed() {
		// Update user keypresses for the user boat [0]
		if win.Pressed(pixelgl.KeyE) {
			userBoat.SailAngle += math.Pi / 60
		} else if win.Pressed(pixelgl.KeyQ) {
			userBoat.SailAngle -= math.Pi / 60
		}
		if win.Pressed(pixelgl.KeyD) {
			userBoat.RudderAngle += math.Pi / 60
		} else if win.Pressed(pixelgl.KeyA) {
			userBoat.RudderAngle -= math.Pi / 60
		}

		// Update AI boats by suppllying their nets with inputs and reading the outputs
		for gi, p := range phenos {
			aiBoat := env.Boats[gi+1]
			aiInputs := env.GetInputs()[gi+1]
			aiOutputs := p.Forward(aiInputs)
			aiBoat.SailAngle = aiOutputs[0] * aiBoat.MaxSailAngle
			aiBoat.RudderAngle = aiOutputs[1] * aiBoat.MaxRudderAngle
		}

		// Step the env
		env.Step(1 / 60.0)

		// Draw sea
		win.Clear(pixel.RGB(0, 105/255.0, 148/255.0))

		// Draw first brain
		genoSprite.Draw(win, pixel.IM.Moved(pixel.V(128, 128)))

		// Draw wind
		{
			cellSize := 50 / zoom
			windImd.Clear()
			windImd.Color = pixel.RGB(0, 135/255.0, 188/255.0)
			if env.OceanOffset.X > cellSize {
				env.OceanOffset.X -= cellSize
			}
			if env.OceanOffset.X < -cellSize {
				env.OceanOffset.X += cellSize
			}
			if env.OceanOffset.Y > cellSize {
				env.OceanOffset.Y -= cellSize
			}
			if env.OceanOffset.Y < -cellSize {
				env.OceanOffset.Y += cellSize
			}
			for x := 0.0; x <= 800; x += 50 {
				for y := 0.0; y <= 800; y += 50 {
					windImd.Push(env.OceanOffset.Scaled(zoom).Add(pixel.V(x, y)))
				}
			}
			windImd.Circle(5, 0)
			windImd.Draw(win)
		}

		// Draw the track
		trackImd.Clear()
		trackImd.Color = pixel.RGB(0, 0, 0).Mul(pixel.Alpha(0.5))
		for _, p := range env.Track {
			trackImd.Push(worldToScreenSpace(pixel.IM).Project(p))
		}
		trackImd.Polygon(3)

		// Draw the current targets. If a target already has a circle, draw a slightly bigger circle
		radiusAdds := make([]float64, len(env.Track))
		for bi := range env.CurrentPoints {
			trackImd.Color = colors[bi]
			pointIdx := env.CurrentPoints[bi]
			trackImd.Push(worldToScreenSpace(pixel.IM).Project(env.Track[pointIdx]))
			trackImd.Circle(env.PointHitDist*zoom+radiusAdds[pointIdx], 2)
			radiusAdds[pointIdx] += 4
		}
		trackImd.Draw(win)

		// Draw boats
		for bi, b := range env.Boats {
			// Hull
			mat := pixel.IM.Scaled(pixel.ZV, 1/BoatSprite.Frame().H())
			mat = worldToScreenSpace(b.TransformMat(mat))
			BoatSprite.DrawColorMask(win, mat, colors[bi])

			// Main sail
			mat = pixel.IM.Scaled(pixel.ZV, 1/SailSprite.Frame().H())
			mat = worldToScreenSpace(b.SailTransformMat(mat))
			SailSprite.DrawColorMask(win, mat, pixel.RGB(0.8, 0, 0.5))

			// Rudder
			mat = pixel.IM.Scaled(pixel.ZV, 0.5/SailSprite.Frame().H())
			mat = worldToScreenSpace(b.RudderTransformMat(mat))
			RudderSprite.DrawColorMask(win, mat, pixel.RGB(0.3, 0, 0.9))

			// Name
			nameText.Clear()
			nameText.WriteString(boatNames[bi])
			orig := worldToScreenSpace(pixel.IM.Moved(b.Pos.Add(pixel.V(0, 0.65)))).Project(pixel.ZV).Sub(pixel.V(15, 0))
			nameText.DrawColorMask(win, pixel.IM.Moved(orig), colors[bi])
		}

		// Update win
		win.Update()
	}
}

func train() {
	// Create counters
	gCounter, sCounter := E.NewAtomicCounter(), E.NewAtomicCounter()

	// Function used to create a new genotype from two parent genotypes
	reprodFunc := func(g1, g2 *E.Genotype) *E.Genotype {
		g := E.NewGenotypeCrossover(g1, g2)
		if rand.Float64() < 0.15 {
			E.AddRandomNeuron(gCounter, g, E.ChooseActivationFrom(activations))
		}
		if rand.Float64() < 0.2 {
			E.AddRandomSynapse(gCounter, g, 0.3, false, 5)
		}
		if rand.Float64() < 0.15 {
			E.PruneRandomSynapse(g)
		}
		for i := 0; i < rand.Intn(4); i++ {
			E.MutateRandomSynapse(g, 0.1)
		}
		return g
	}

	// Create a population of empty genotypes. These are all copies of one genotype so that they share the same input and ouput node ids
	initialGenotype := E.NewGenotype(gCounter, 9, 2, E.AcLin, E.AcTanh)
	pop := repeated(func() *E.Agent { return E.NewAgent(E.NewGenotypeCopy(initialGenotype)) }, targetPopSize)

	// Distance threshold for speciation
	distThresh := 2.0

	// Used to keep track of best performer
	bestFitness := math.Inf(-1)
	bestGeno := pop[0].Genotype

	// Generational loop
	for gen := 1; gen <= maxGenerations; gen++ {
		// Calculate fitness for each agent
		// This is the total fitnesss of the agent over multiple tracks
		// Start by zeroing fitnesses
		for _, a := range pop {
			a.Fitness = 0
		}
		// Generate all phenotypes
		phenos := apply(func(a *E.Agent) *E.Phenotype { return E.NewPhenotype(a.Genotype) }, pop)
		// For each track
		for sim := 0; sim < simsPerGen; sim++ {
			// Create a new set of boats for the ai
			boats := repeated(NewDefaultBoat, len(pop))
			// Create an environment with a track for the boats to race on
			env := BasicEnv(NewTrack(12, 10), boats...)
			// For each step in the sim
			for step := 0; step < simSteps; step++ {
				// Run the AI and let it control the boat
				inputs := env.GetInputs()
				for ai := range pop {
					outs := phenos[ai].Forward(inputs[ai])
					boats[ai].SailAngle = outs[0] * boats[ai].MaxSailAngle
					boats[ai].RudderAngle = outs[1] * boats[ai].MaxRudderAngle
				}
				// Step the env
				env.Step(1 / 60.0)
			}
			// Read the fitnesses for each boat and add them to the current fitnesses
			fitnesses := env.GetFitnesses()
			for ai, a := range pop {
				a.Fitness += fitnesses[ai]
			}
		}

		// Find the best genotype with best fitness. Also scale all fitnesses by 1/num_repeats
		bestFitness = math.Inf(-1)
		bestGeno = nil
		for _, a := range pop {
			a.Fitness /= float64(simsPerGen)
			if a.Fitness > bestFitness {
				bestFitness = a.Fitness
				bestGeno = a.Genotype
			}
		}

		// Speciate the populaion
		specPop := E.Speciate(sCounter, pop, distThresh, false, E.GeneticDistance(1, 0.4))

		// Update the species threshold to get closer to target species
		if len(specPop) < targetSpecies {
			distThresh /= 1.1
		}
		if len(specPop) > targetSpecies {
			distThresh *= 1.1
		}

		// Sometimes print out some log info
		if gen%10 == 0 {
			fmt.Printf("Generation %v, Species %v, Best Fitness %v\n", gen, len(specPop), bestFitness)
		}

		// Calculate how many offspring each species are allowed
		allowedOffspring := E.CalculateOffspring(specPop, targetPopSize)

		// Create the next generation from the previous speciated population
		pop = E.Repopulate(specPop, allowedOffspring, reprodFunc, E.ProbabilisticSelection)
	}

	fmt.Println("Saving genotype")
	// Save the best genotype to the specified file
	if f, err := os.Create(genoFileNames); err != nil {
		panic(err)
	} else {
		enc := json.NewEncoder(f)
		enc.Encode(bestGeno)
		f.Close()
	}
	fmt.Println("Done")
}

// Load all sprites from disk
func loadSprites() {
	BoatSprite = must(loadFullSprite("sprites/boat.png"))
	SailSprite = must(loadFullSprite("sprites/sail.png"))
	RudderSprite = must(loadFullSprite("sprites/rudder.png"))
}

// util to load a png file to a pixel.Picture
func loadPicture(path string) (pixel.Picture, error) {
	//file, err := os.Open(path)
	file, err := spritesFS.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return nil, err
	}
	return pixel.PictureDataFromImage(img), nil
}

// util to load a png file to a pixel.Sprite
func loadFullSprite(path string) (*pixel.Sprite, error) {
	if pic, err := loadPicture(path); err != nil {
		return nil, err
	} else {
		return pixel.NewSprite(pic, pic.Bounds()), nil
	}
}

func readActivationList(colonSepAcs string) []E.Activation {
	rawAcs := strings.Split(colonSepAcs, ":")
	acs := make([]E.Activation, len(rawAcs))
	for acI, ac := range rawAcs {
		acA := E.Activation(ac)
		switch acA {
		case E.AcLin, E.AcReLU, E.AcReLUM, E.AcReLn, E.AcSig, E.AcStep, E.AcTanh:
			acs[acI] = acA
		default:
			panic("invalid activation: " + ac)
		}
	}
	return acs
}
