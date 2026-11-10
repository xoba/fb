**free
// Print labels for an in-memory file catalog.
ctl-opt dftactgrp(*no) main(main);

dcl-ds entry_t qualified template;
  name varchar(20);
  bytes int(10);
end-ds;

dcl-proc main;
  dcl-ds files likeds(entry_t) dim(2);
  dcl-s i int(10);
  dcl-s total int(10) inz(0);

  files(1).name = 'notes.md';
  files(1).bytes = 120;
  files(2).name = 'letter.eml';
  files(2).bytes = 240;

  for i = 1 to %elem(files);
    dsply (files(i).name + ': ' + %char(files(i).bytes) + ' bytes');
    total += files(i).bytes;
  endfor;
  dsply ('total: ' + %char(total) + ' bytes');
end-proc;
