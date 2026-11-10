// A desk stand with one slot per catalog entry, each labelled on the front.
labels = ["notes.md", "letter.eml"];
card = [60, 4, 40];
gap = 10;
depth = len(labels) * (card[1] + gap) + gap;

module slot(i) {
    translate([5, gap + i * (card[1] + gap), 5])
        cube(card + [0, 0, 1]);
}

difference() {
    cube([70, depth, 15]);
    for (i = [0 : len(labels) - 1]) slot(i);
}

for (i = [0 : len(labels) - 1])
    translate([8, gap + i * (card[1] + gap) - 3, 15])
        linear_extrude(height = 1)
            text(labels[i], size = 3);
