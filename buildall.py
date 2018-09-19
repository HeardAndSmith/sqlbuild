# use with python buildall.py | /bin/bash -ex

# wget -q https://registry.hub.docker.com/v1/repositories/microsoft/mssql-server-linux/tags -O -  | sed -e 's/[][]//g' -e 's/"//g' -e 's/ //g' | tr '}' '\n'  | awk -F: '{print $3}'
tags = set([
  '2017-CU1',
  '2017-CU10',
  '2017-CU2',
  '2017-CU3',
  '2017-CU4',
  '2017-CU5',
  '2017-CU6',
  '2017-CU7',
  '2017-CU8',
  '2017-CU9',
  '2017-CU9-GDR2',
  '2017-GA',
  '2017-GDR',
  '2017-GDR2',
  '2017-latest',
])
tags.add('latest')
repository = 'hslaw/mssql-build'

for tag in tags:
  print(' '.join([
    "docker", "build",
    "--pull",
    "--build-arg", "MSSQL_BUILD_TAG=" + tag,
    "-t", repository + ":" + tag,
    "--target", "build",
    "."
  ]))
print('docker push ' + repository)
