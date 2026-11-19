# Writes an assembly STEP with OpenCASCADE (FreeCAD): a plate and the same
# pin placed twice, into io/testdata/occ_assembly.step, and prints each
# solid's volume and centre as FreeCAD reads it back.
import os
import sys

bin_dir = os.path.dirname(sys.executable)
sys.path += [bin_dir, os.path.join(os.path.dirname(bin_dir), "lib")]

import FreeCAD  # noqa: E402
import Import  # noqa: E402
import Part  # noqa: E402
from FreeCAD import Placement, Rotation, Vector as V  # noqa: E402

out = sys.argv[1]
doc = FreeCAD.newDocument("asm")
asm = doc.addObject("App::Part", "Assembly")

plate = doc.addObject("App::Part", "Plate")
pb = doc.addObject("Part::Feature", "PlateBody")
pb.Shape = Part.makeBox(40, 30, 10).cut(Part.makeCylinder(5, 14, V(20, 15, -2)))
plate.addObject(pb)
asm.addObject(plate)

pin = doc.addObject("App::Part", "Pin")
pinb = doc.addObject("Part::Feature", "PinBody")
pinb.Shape = Part.makeCylinder(5, 30)
pin.addObject(pinb)
pin.Placement = Placement(V(20, 15, -5), Rotation())
asm.addObject(pin)

link = doc.addObject("App::Link", "Pin2")
link.LinkedObject = pin
link.Placement = Placement(V(60, 0, 0), Rotation(V(1, 0, 0), 90))
asm.addObject(link)
doc.recompute()

Import.export([asm], out)
shape = Part.Shape()
shape.read(out)
for s in shape.Solids:
    c = s.CenterOfMass
    print("solid %r %.4f %.4f %.4f" % (s.Volume, c.x, c.y, c.z))
