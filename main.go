// Forma is a parametric 3D CAD program: constrained sketches, B-rep solids
// with analytic geometry, OpenGL 4.1, Dear ImGui.
package main

import (
	"log"
	"runtime"

	"forma/app"
)

func init() { runtime.LockOSThread() }

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
