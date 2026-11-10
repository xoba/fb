require "fileinto";

# File catalog notices from the example.com archive into their own folder.
if address :domain :is "from" "example.com" {
    if header :contains "subject" "catalog" {
        fileinto "Catalog";
        stop;
    }
}

# Large messages go to a review folder instead of the inbox.
if size :over 240K {
    fileinto "Review";
}
