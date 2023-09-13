package main

import (
	"math"
	"math/rand"

	"github.com/faiface/pixel"
)

type Boat struct {
	// Updated by sim
	Pos    pixel.Vec // Position
	Rot    float64   // Rotation
	Vel    pixel.Vec // Velocity
	RotVel float64   // Rotational velocity

	// Boat Params
	SailArea       float64 // The area of the sail in the wind. 1 works fine
	DragForward    float64 // The drag when going forwards. 0-inf
	DragBack       float64 // The drag when going backwards. 0-inf
	DragPerp       float64 // The drag when going sideways. 0-inf
	AngularDrag    float64 // The angular drag of the boat
	RudderForce    float64 // The force the rudder puts on the boat when going 1m/s
	Length         float64 // The ength of the boat
	MaxSailAngle   float64 // The max sail angle
	MaxRudderAngle float64 // The max rudder angle

	// Updated by pilot
	SailAngle   float64 // CONTROL VAR: The angle of the sail -pi/2:pi/2
	RudderAngle float64 // CONTROL VAR: The angle of the rudder -pi/2:pi/2
}

// Create a boat with decent physics
func NewDefaultBoat() *Boat {
	return &Boat{
		SailArea:       1,
		DragForward:    0.1,
		DragBack:       2,
		DragPerp:       4,
		AngularDrag:    20,
		RudderForce:    0.3,
		Length:         1,
		MaxSailAngle:   math.Pi / 2,
		MaxRudderAngle: math.Pi / 2,
	}
}

func (b *Boat) TransformMat(mat pixel.Matrix) pixel.Matrix {
	return mat.Scaled(pixel.ZV, b.Length).Rotated(pixel.ZV, b.Rot).Moved(b.Pos)
}
func (b *Boat) SailTransformMat(mat pixel.Matrix) pixel.Matrix {
	return mat.Scaled(pixel.ZV, b.Length).Rotated(pixel.ZV, b.Rot+b.SailAngle).Moved(b.Pos)
}
func (b *Boat) RudderTransformMat(mat pixel.Matrix) pixel.Matrix {
	return mat.Scaled(pixel.ZV, b.Length).Rotated(pixel.ZV, b.Rot+b.RudderAngle).Moved(b.Pos.Add(pixel.V(0, -b.Length/2).Rotated(b.Rot)))
}

type Env struct {
	WindVec             pixel.Vec
	Boats               []*Boat
	WindForceMultiplier float64
	OceanOffset         pixel.Vec
	Track               []pixel.Vec
	PointHitDist        float64
	CurrentPoints       []int
	NumWaypointsReached []int
}

func NewTrack(distance float64, points int) []pixel.Vec {
	track := make([]pixel.Vec, points)
	for i := range track {
		track[i] = pixel.V(0, math.Sqrt(rand.Float64())*distance).Rotated(rand.Float64() * math.Pi * 2)
	}
	return track
}

// Must make sure
func reflectVec(dir, normal pixel.Vec) pixel.Vec {
	dottedF := dir.Dot(normal)
	return dir.Sub(normal.Scaled(2 * dottedF))
}

func (e *Env) Step(dt float64) {
	//Ocean offset (only got visuals)
	e.OceanOffset = e.OceanOffset.Add(e.WindVec.Scaled(dt))
	// Update motion of boats
	for bi, b := range e.Boats {
		b.RudderAngle = clamp(b.RudderAngle, -b.MaxRudderAngle, b.MaxRudderAngle)
		b.SailAngle = clamp(b.SailAngle, -b.MaxSailAngle, b.MaxSailAngle)
		// Calculate force of wind on sail
		sailNormal := pixel.V(1, 0).Rotated(b.SailAngle + b.Rot)
		// ensure sailnormal is towards wind
		relativeWindVec := e.WindVec.Sub(b.Vel)
		if sailNormal.Dot(relativeWindVec.Unit()) > 0 {
			sailNormal = sailNormal.Scaled(-1)
		}
		reflectedRelativeWindVec := reflectVec(relativeWindVec, sailNormal)
		windChangeVec := reflectedRelativeWindVec.Sub(relativeWindVec)
		windSailForce := windChangeVec.Scaled(-b.SailArea * sailNormal.Scaled(-1).Dot(relativeWindVec.Unit())) // Multiplied by visible sail area
		// Add forces
		b.Vel = b.Vel.Add(windSailForce.Scaled(dt * e.WindForceMultiplier))

		// Apply drag (different in forward and sideways direction)
		paraVec, perpVec := pixel.V(0, 1).Rotated(b.Rot), pixel.V(1, 0).Rotated(b.Rot)
		velPara, velPerp := b.Vel.Dot(paraVec), b.Vel.Dot(perpVec)
		paraDrag := 0.0
		if velPara > 0 {
			paraDrag = b.DragForward
		} else {
			paraDrag = b.DragBack
		}
		velAdjPara, velAdjPerp := velPara*paraDrag*dt, velPerp*b.DragPerp*dt
		if math.Abs(velAdjPara) > math.Abs(velPara) {
			velAdjPara = velPara
		}
		if math.Abs(velAdjPerp) > math.Abs(velPerp) {
			velAdjPerp = velPerp
		}
		velPara, velPerp = velPara-velAdjPara, velPerp-velAdjPerp
		b.Vel = paraVec.Scaled(velPara).Add(perpVec.Scaled(velPerp))

		// Apply rudder + rudder drag
		b.RotVel += -b.RudderAngle * velPara * b.RudderForce
		rotVelAdj := b.RotVel * b.AngularDrag * dt
		if math.Abs(rotVelAdj) > math.Abs(b.RotVel) {
			rotVelAdj = b.RotVel
		}
		b.RotVel -= rotVelAdj

		// Apply rotation of velocity
		b.Vel = b.Vel.Rotated(b.RotVel * dt)

		// Add velocities
		b.Pos = b.Pos.Add(b.Vel.Scaled(dt))
		b.Rot += b.RotVel * dt

		// Check for update next point
		if b.Pos.Sub(e.Track[e.CurrentPoints[bi]]).Len() <= e.PointHitDist {
			e.CurrentPoints[bi] = (e.CurrentPoints[bi] + 1) % len(e.Track)
			e.NumWaypointsReached[bi] += 1
		}
	}
}

func (e *Env) GetFitnesses() []float64 {
	fitnesses := make([]float64, len(e.Boats))
	for i, b := range e.Boats {
		currentPoint := e.Track[e.CurrentPoints[i]]
		//previousPoint := e.Track[e.CurrentPoints[(i-1)%len(e.CurrentPoints)]]
		distToCurrentPoint := b.Pos.Sub(currentPoint).Len()
		//distPropPrevPoint := b.Pos.Sub().Len()
		fitnesses[i] = (1 / (distToCurrentPoint + 1)) + float64(e.NumWaypointsReached[i])
	}
	return fitnesses
}

func (e *Env) GetInputs() [][]float64 {
	inputs := make([][]float64, len(e.Boats))
	for i, b := range e.Boats {
		currPointVec := e.Track[e.CurrentPoints[i]].Sub(b.Pos)
		inputs[i] = []float64{
			b.SailAngle / b.MaxSailAngle,                                     // -1:1
			b.RudderAngle / b.MaxRudderAngle,                                 // -1:1
			TransCenteredRolloff(b.Vel.Dot(pixel.V(0, 1).Rotated(b.Rot)), 5), // -1:1
			TransCenteredRolloff(b.Vel.Dot(pixel.V(1, 0).Rotated(b.Rot)), 5), // -1:1
			TransCenteredRolloff(b.RotVel, math.Pi/2),                        // -1:1
			TransCenteredRolloff(e.WindVec.Dot(pixel.V(0, 1).Rotated(b.Rot)), 5),
			TransCenteredRolloff(e.WindVec.Dot(pixel.V(1, 0).Rotated(b.Rot)), 5),
			TransCenteredRolloff(currPointVec.Dot(pixel.V(0, 1).Rotated(b.Rot)), 5),
			TransCenteredRolloff(currPointVec.Dot(pixel.V(1, 0).Rotated(b.Rot)), 5),
		}
	}
	return inputs
}
