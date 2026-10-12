<?php
$files = ["notes.md", "letter.eml"];
foreach ($files as $name) {
    echo "Preview: " . htmlspecialchars($name, ENT_QUOTES, "UTF-8") . "\n";
}
