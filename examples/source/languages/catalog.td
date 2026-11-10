// Describe catalog file kinds as TableGen records.
class FileKind<string ext, int maxBytes> {
  string Extension = ext;
  int MaxBytes = maxBytes;
  bit Text = 1;
}

def Notes  : FileKind<".md", 120>;
def Letter : FileKind<".eml", 240>;

let Text = 0 in
def Archive : FileKind<".zip", 4096>;

multiclass Sized<int base> {
  def _small : FileKind<".txt", base>;
  def _large : FileKind<".txt", !mul(base, 4)>;
}

defm Plain : Sized<100>;
