// Package fea is Forma's static stress analysis: a solid filled with
// quadratic tetrahedra, linear elasticity, faces held fixed and loaded,
// and the displacement, von Mises stress and safety factor that come out.
// Units are the design's: millimetres, newtons, megapascals (N/mm²).
package fea

// Material is a linear elastic, isotropic material.
type Material struct {
	Name    string
	E       float64 // Young's modulus, MPa
	Nu      float64 // Poisson's ratio
	Yield   float64 // yield (or for plastics, tensile) strength, MPa
	Density float64 // g/cm³
}

// Materials are common ones, typical values (a datasheet's beat these;
// printed plastics vary with infill and direction, and are weaker across
// layers).
var Materials = []Material{
	{"Steel (mild)", 210000, 0.30, 250, 7.85},
	{"Stainless steel 304", 193000, 0.29, 215, 8.0},
	{"Aluminium 6061-T6", 69000, 0.33, 276, 2.70},
	{"Brass", 100000, 0.34, 200, 8.5},
	{"PLA (printed)", 3500, 0.36, 50, 1.24},
	{"PETG (printed)", 2100, 0.40, 45, 1.27},
	{"ABS (printed)", 2300, 0.35, 40, 1.04},
	{"Nylon PA12", 1700, 0.40, 48, 1.01},
	{"Oak (along the grain)", 11000, 0.35, 50, 0.75},
}

// MaterialNamed finds a material by name (the first if there is none).
func MaterialNamed(name string) Material {
	for _, m := range Materials {
		if m.Name == name {
			return m
		}
	}
	return Materials[0]
}
