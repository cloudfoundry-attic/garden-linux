set -e -x

cd /opt/warden 

tars=$(find . -name '*.tar')
for path in $tars; do
  dirname=$(echo $path | sed 's/\.\/\([a-zA-Z_\-]*\)\.tar/\1/')
  mkdir $dirname
  tar -xf $path -C ${PWD}/${dirname}
done 
rm *.tar

cd -

