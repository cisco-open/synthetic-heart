for file in $( find . -type f -name 'requirements.txt' );
  do pip3 install -r $file $1
  done