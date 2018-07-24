prefix='export '; file=/tmp/buildEnv; newfile=/tmp/exportBuildEnv; 
while read -r line; do
escapeQuote=`echo $line | sed 's/"/\\\"/g'`;
newline=`echo $escapeQuote | sed 's/\([A-Z_][A-Z0-9_]*\)=\(.*\)/\1="\2"/'`;
echo ${prefix}$newline; 
done < $file > $newfile;
