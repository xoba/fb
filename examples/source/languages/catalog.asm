; Print one catalog label and exit (Linux x86-64, NASM syntax).
        global  _start

        section .data
label:  db      "notes.md: 120 bytes", 10
len:    equ     $ - label

        section .text
_start:
        mov     rax, 1          ; write
        mov     rdi, 1          ; stdout
        lea     rsi, [rel label]
        mov     rdx, len
        syscall
        mov     rax, 60         ; exit
        xor     edi, edi
        syscall
