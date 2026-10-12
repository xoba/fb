files <- data.frame(
  name = c("notes.md", "letter.eml"),
  bytes = c(120, 240)
)
print(files)
cat("Total:", sum(files$bytes), "bytes\n")
