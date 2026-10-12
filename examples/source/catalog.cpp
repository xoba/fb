#include <array>
#include <iostream>
#include <string_view>

int main() {
    constexpr std::array<std::string_view, 2> kinds{"document", "message"};
    for (auto kind : kinds) std::cout << kind << '\n';
}
