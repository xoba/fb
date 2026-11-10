; Print one catalog label on Linux x86-64, then exit.
        section .data
label:  db      "notes.md: 120 bytes", 10
len:    equ     $ - label

        section .text
        global  _start
_start:
        mov     rax, 1          ; write
        mov     rdi, 1          ; stdout
        lea     rsi, [rel label]
        mov     rdx, len
        syscall

        mov     rax, 60         ; exit
        xor     edi, edi
        syscall
