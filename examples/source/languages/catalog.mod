# Choose which catalog files fit in a small cache.
set FILES;
param bytes {FILES} > 0;
param capacity >= 0;

var keep {FILES} binary;

maximize Kept: sum {f in FILES} bytes[f] * keep[f];
subject to Fits: sum {f in FILES} bytes[f] * keep[f] <= capacity;

data;
set FILES := 'notes.md' 'letter.eml';
param bytes := 'notes.md' 120 'letter.eml' 240;
param capacity := 300;
