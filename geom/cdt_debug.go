package geom

// CDTStats counts trouble in Triangulate (for tests and debugging).
var CDTStats struct {
	Unlocated   int // points that fell outside every triangle
	Unrecovered int // constraint segments that could not be made edges
}
