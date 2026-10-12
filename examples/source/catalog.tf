# Values only: this example declares no providers or resources.
locals {
  catalog = ["notes.md", "letter.eml"]
}

output "file_count" {
  description = "Number of entries in the example catalog."
  value       = length(local.catalog)
}
