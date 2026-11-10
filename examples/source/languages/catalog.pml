/* Two writers add catalog entries under a shared lock. */
#define N 2

bool locked = false;
byte count = 0;
int total = 0;

inline acquire() { atomic { !locked -> locked = true } }
inline release() { locked = false }

proctype Writer(int bytes) {
    acquire();
    count++;
    total = total + bytes;
    release()
}

init {
    atomic {
        run Writer(120);   /* notes.md */
        run Writer(240)    /* letter.eml */
    };
    (_nr_pr == 1) -> assert(count == N && total == 360)
}
