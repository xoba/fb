package Browse is
   type File_Kind is (Document, Image, Message);
   function Label (Kind : File_Kind) return String;
end Browse;
