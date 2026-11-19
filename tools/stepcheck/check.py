# Reads STEP files with OpenCASCADE (FreeCAD's Part module) and prints, per
# solid: "solid <file> <valid> <volume> <area> <faces>", then any problems
# BRepCheck finds. Run with FreeCAD's own python.exe:
#   "C:\Program Files\FreeCAD 1.1\bin\python.exe" tools/stepcheck/check.py a.step b.step
import os
import sys

bin_dir = os.path.dirname(sys.executable)
sys.path += [bin_dir, os.path.join(os.path.dirname(bin_dir), "lib")]

import FreeCAD  # noqa: E402,F401
import Part  # noqa: E402

for path in sys.argv[1:]:
    shape = Part.Shape()
    try:
        shape.read(path)
    except Exception as e:  # noqa: BLE001
        print("error", path, e)
        continue
    if not shape.Solids:
        print("nosolid", path, shape.ShapeType, len(shape.Faces))
    for s in shape.Solids:
        c = s.CenterOfMass
        print("solid", path, s.isValid(), repr(s.Volume), repr(s.Area), len(s.Faces), repr(c.x), repr(c.y), repr(c.z))
        if not s.isValid():
            try:
                s.check(True)
            except Exception as e:  # noqa: BLE001
                print("  check:", str(e).replace("\n", " | ")[:2000])
