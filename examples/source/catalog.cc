#include <iostream>
#include <vector>

int main() {
    const std::vector<const char*> names{"notes.md", "letter.eml"};
    for (const auto* name : names) std::cout << name << '\n';
}
