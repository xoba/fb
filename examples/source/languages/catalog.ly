\version "2.24.0"
% A short chime played when the file catalog opens.
\header {
  title = "Catalog Chime"
}

\relative c' {
  \clef treble
  \key g \major
  \time 3/4
  g4 b d | g2. |
  fis4( e) d | g,2. \bar "|."
}
