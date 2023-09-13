compile-from-linux:
	go build -o ./bin/neat-sail .
	CGO_ENABLED=1 GOARCH=386 GOOS=windows CXX=i686-w64-mingw32-g++ CC=i686-w64-mingw32-gcc go build -o ./bin/neat-sail.exe .