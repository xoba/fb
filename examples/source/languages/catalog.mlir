// Sum the byte sizes of an in-memory file catalog.
func.func @total_bytes(%sizes: memref<2xi64>) -> i64 {
  %c0 = arith.constant 0 : index
  %c1 = arith.constant 1 : index
  %c2 = arith.constant 2 : index
  %zero = arith.constant 0 : i64
  %sum = scf.for %i = %c0 to %c2 step %c1 iter_args(%acc = %zero) -> (i64) {
    %size = memref.load %sizes[%i] : memref<2xi64>
    %next = arith.addi %acc, %size : i64
    scf.yield %next : i64
  }
  return %sum : i64
}
