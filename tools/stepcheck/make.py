# Writes STEP files made by OpenCASCADE (FreeCAD's Part module) into
# io/testdata, with their volumes in volumes.txt, for the import tests:
#   "C:\Program Files\FreeCAD 1.1\bin\python.exe" tools/stepcheck/make.py io/testdata
import math
import os
import sys

bin_dir = os.path.dirname(sys.executable)
sys.path += [bin_dir, os.path.join(os.path.dirname(bin_dir), "lib")]

import FreeCAD  # noqa: E402
import Part  # noqa: E402
from FreeCAD import Vector as V  # noqa: E402

out = sys.argv[1]
parts = {}

box = Part.makeBox(30, 20, 10)
parts["occ_fillet_box"] = box.makeFillet(2, box.Edges)

plate = Part.makeBox(40, 30, 8, V(-20, -15, 0))
tilted = Part.makeCylinder(4, 40, V(0, 0, -10), V(0.5, 0, 1))
parts["occ_oblique_hole"] = plate.cut(tilted)

c1 = Part.makeCylinder(5, 30, V(0, 0, -15), V(0, 0, 1))
c2 = Part.makeCylinder(4, 30, V(-15, 0, 0), V(1, 0, 0))
parts["occ_cross"] = c1.fuse(c2)

b = Part.makeBox(20, 20, 20)
s = Part.makeSphere(6, V(10, 10, 20))
t = Part.makeTorus(6, 2, V(10, 10, 0))
parts["occ_bumps"] = b.fuse(s).cut(t)

c = Part.makeCone(8, 3, 10)
parts["occ_cone_chamfer"] = c.makeChamfer(1, [e for e in c.Edges if e.Curve.__class__.__name__ == "Circle"])

with open(os.path.join(out, "volumes.txt"), "w") as f:
    for name, shp in parts.items():
        path = os.path.join(out, name + ".step")
        shp.exportStep(path)
        kinds = sorted({fc.Surface.__class__.__name__ for fc in shp.Faces})
        ekinds = sorted({(e.Curve.__class__.__name__ if not e.Degenerated else "degenerate") for e in shp.Edges})
        f.write("%s %r %d\n" % (name, shp.Volume, len(shp.Faces)))
        print(name, shp.isValid(), shp.Volume, len(shp.Faces), kinds, ekinds)
