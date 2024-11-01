# http://{{WEB}}/red_100x100_changed_after_fetch.png (will become red_10000x1000.png file content at next fetch)
GET http://{{TARGET}}:8000/img/687474703a2f2f{{WEB_HEX}}2f7265645f313030783130305f6368616e6765645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 200
[Asserts]
bytes count == 692
sha256 == hex,449d4fc873c1d6d77703f1f0253caecbaac812e3066a3e28402c65370085dae7;
header "Content-Type" == "image/png"

# http://{{WEB}}/red_100x100_removed_after_fetch.png (until next fetch)
GET http://{{TARGET}}:8000/img/687474703a2f2f{{WEB_HEX}}2f7265645f313030783130305f72656d6f7665645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 200
[Asserts]
bytes count == 692
sha256 == hex,449d4fc873c1d6d77703f1f0253caecbaac812e3066a3e28402c65370085dae7;
header "Content-Type" == "image/png"

# http://{{WEB}}/red_100x100_converted_after_fetch.png (will be converted at next fetch)
GET http://{{TARGET}}:8000/img/687474703a2f2f{{WEB_HEX}}2f7265645f313030783130305f636f6e7665727465645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 200
[Asserts]
bytes count == 692
sha256 == hex,449d4fc873c1d6d77703f1f0253caecbaac812e3066a3e28402c65370085dae7;
header "Content-Type" == "image/png"

# http://{{WEB}}/red_100x100_blocked_after_fetch.png (immediate effect)
GET http://{{TARGET}}:8000/img/687474703a2f2f{{WEB_HEX}}2f7265645f313030783130305f626c6f636b65645f61667465725f66657463682e706e67
[Options]
http2: {{HTTP2}}
HTTP/1.1 404
