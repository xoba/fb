;; Sum the sizes of a catalog whose entries are stored in linear memory.
(module
  (memory (export "memory") 1)
  (data (i32.const 0) "\78\00\00\00\f0\00\00\00")

  (func $total (export "total") (param $count i32) (result i32)
    (local $i i32)
    (local $sum i32)
    (block $done
      (loop $next
        (br_if $done (i32.ge_u (local.get $i) (local.get $count)))
        (local.set $sum
          (i32.add (local.get $sum)
                   (i32.load (i32.mul (local.get $i) (i32.const 4)))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $next)))
    (local.get $sum))
)
