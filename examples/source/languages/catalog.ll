; Total the byte sizes of an in-memory file catalog: notes.md and letter.eml.
@names = private unnamed_addr constant [20 x i8] c"notes.md\00letter.eml\00"

define i64 @total_bytes([2 x i64] %sizes) {
entry:
  %notes = extractvalue [2 x i64] %sizes, 0
  %letter = extractvalue [2 x i64] %sizes, 1
  %total = add nuw i64 %notes, %letter
  ret i64 %total
}

define i32 @main() {
entry:
  %total = call i64 @total_bytes([2 x i64] [i64 120, i64 240])
  %large = icmp ugt i64 %total, 256
  br i1 %large, label %big, label %small

big:
  ret i32 1

small:
  ret i32 0
}
