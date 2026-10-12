cmake_minimum_required(VERSION 3.20)
project(ExampleCatalog LANGUAGES NONE)

set(EXAMPLE_FILES notes.md letter.eml)
list(LENGTH EXAMPLE_FILES EXAMPLE_COUNT)
message(STATUS "Catalog contains ${EXAMPLE_COUNT} example names")
