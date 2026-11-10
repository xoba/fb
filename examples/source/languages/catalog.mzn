% Pack catalog files onto the fewest disks that hold them.
int: n = 2;
array[1..n] of string: names = ["notes.md", "letter.eml"];
array[1..n] of int: bytes = [120, 240];
int: capacity = 256;

array[1..n] of var 1..n: disk;
var 1..n: used = max(disk);

constraint forall(d in 1..n)(
  sum(i in 1..n)(bytes[i] * (disk[i] = d)) <= capacity
);

solve minimize used;

output [names[i] ++ " -> disk " ++ show(disk[i]) ++ "\n" | i in 1..n];
