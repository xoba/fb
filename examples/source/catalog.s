// A small AArch64 function: return the number of sample files.
.text
.p2align 2
.globl _catalog_count
_catalog_count:
    mov w0, #2
    ret
