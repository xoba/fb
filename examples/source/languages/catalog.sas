/* Print labels for an in-memory file catalog. */
data catalog;
    length name $ 20 label $ 40;
    input name $ bytes;
    label = catx(' ', cats(name, ':'), bytes, 'bytes');
    datalines;
notes.md 120
letter.eml 240
;
run;

proc sort data=catalog;
    by descending bytes;
run;

proc print data=catalog noobs;
    var label;
run;

proc means data=catalog sum;
    var bytes;
run;
