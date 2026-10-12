program catalog
  implicit none
  integer, parameter :: documents = 3, messages = 2
  integer :: total

  total = documents + messages
  print *, 'Example files:', total
end program catalog
