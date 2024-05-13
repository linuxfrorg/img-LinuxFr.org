# http://nginx/red_100x100_changed_after_fetch.png (became red_10000x1000.png file content)
GET http://{{TARGET}}:8000/img/687474703a2f2f6e67696e782f7265645f313030783130305f6368616e6765645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 200
[Asserts]
bytes count == 317124
sha256 == hex,1629535232f1b74c6e05b9284bf90034ae4e76a5c803437e75ac4c6b9ce4fc26;
header "Content-Type" == "image/png"

# http://nginx/red_100x100_removed_after_fetch.png
GET http://{{TARGET}}:8000/img/687474703a2f2f6e67696e782f7265645f313030783130305f72656d6f7665645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 404

# http://nginx/red_100x100_converted_after_fetch.png
GET http://{{TARGET}}:8000/img/687474703a2f2f6e67696e782f7265645f313030783130305f636f6e7665727465645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 200
[Asserts]
bytes count == 169
sha256 == hex,ecebb0a413120b65c28ddb0d9604659376cb0f45a3dceb597c1336bce61cb20e;
header "Content-Type" == "image/png" # not really

# http://nginx/red_100x100_blocked_after_fetch.png
GET http://{{TARGET}}:8000/img/687474703a2f2f6e67696e782f7265645f313030783130305f626c6f636b65645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 404
