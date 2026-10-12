#ifndef EXAMPLE_CATALOG_HPP
#define EXAMPLE_CATALOG_HPP

#include <cstddef>
#include <string_view>

struct CatalogEntry {
    std::string_view name;
    std::size_t bytes;
};

#endif
