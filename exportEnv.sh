# Script to export ENV

prefix='export '; file=/tmp/test; tmpfile=/tmp/test_tmp; newfile=/tmp/test_export; env | grep -vi PS1 > $file && while read -r line; do echo $line | sed 's/"/\\\"/g' | sed 's/$/\\n/' | tr -d '\n' | sed 's/..$//' | sed 's/\([A-Za-z_][A-Za-z0-9_]*\)=\(.*\)/\1="\2"/' | sed 's/^/export /'; done < $file > $tmpfile; mv $tmpfile $newfile;

# Script to source ENV
while ! [ -f /tmp/env_export ] && [ $(( $(date +'%s') - $START )) -lt 10 ]; do sleep 1; done; . /tmp/env_export;
