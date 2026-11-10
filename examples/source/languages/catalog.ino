// Print labels for an in-memory file catalog over serial.
struct Entry {
  const char *name;
  unsigned int bytes;
};

const Entry files[] = {
  {"notes.md", 120},
  {"letter.eml", 240},
};

void setup() {
  Serial.begin(9600);
  for (const Entry &entry : files) {
    Serial.print(entry.name);
    Serial.print(": ");
    Serial.print(entry.bytes);
    Serial.println(" bytes");
  }
}

void loop() {
}
