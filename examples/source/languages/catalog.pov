// Two boxes standing in for the catalog entries, sized by their bytes.
#include "colors.inc"

camera {
  location <0, 2, -6>
  look_at <0, 0.5, 0>
}

light_source { <4, 6, -4> color White }

plane { y, 0 pigment { checker color Gray80 color Gray60 } }

#declare Sizes = array[2] { 120, 240 };

#for (I, 0, 1)
  box {
    <-0.5, 0, -0.5>, <0.5, Sizes[I] / 120, 0.5>
    pigment { color rgb <0.3, 0.5, 0.8> }
    translate <I * 1.5 - 0.75, 0, 0>
  }
#end

text {
  ttf "timrom.ttf" "notes.md  letter.eml" 0.1, 0
  scale 0.3 translate <-1.5, 2.3, 0>
  pigment { color White }
}
