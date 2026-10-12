#include <stdio.h>

int main(void) {
    const char *files[] = {"notes.md", "letter.eml"};
    for (unsigned int i = 0; i < sizeof(files) / sizeof(files[0]); ++i) {
        printf("Preview: %s\n", files[i]);
    }
    return 0;
}
