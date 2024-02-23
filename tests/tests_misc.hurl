GET http://{{TARGET}}:8000
HTTP 404

GET http://{{TARGET}}:8000
[Options]
http2: true
HTTP/1.1 404

POST http://{{TARGET}}:8000
HTTP 404

POST http://{{TARGET}}:8000
[Options]
http2: true
HTTP/1.1 404

UNKNOWN http://{{TARGET}}:8000
HTTP 404

UNKNOWN http://{{TARGET}}:8000
[Options]
http2: true
HTTP/1.1 404

GET http://{{TARGET}}:8000/
HTTP 404

GET http://{{TARGET}}:8000/
[Options]
http2: true
HTTP/1.1 404

POST http://{{TARGET}}:8000/
HTTP 404

POST http://{{TARGET}}:8000/
[Options]
http2: true
HTTP/1.1 404

GET http://{{TARGET}}:8000/unknown
HTTP 404

GET http://{{TARGET}}:8000/unknown
[Options]
http2: true
HTTP/1.1 404

GET http://{{TARGET}}:8000/status
HTTP 200
[Asserts]
body contains "OK"

GET http://{{TARGET}}:8000/status
[Options]
http2: true
HTTP/1.1 200
[Asserts]
body contains "OK"

POST http://{{TARGET}}:8000/status
HTTP 405

POST http://{{TARGET}}:8000/status
[Options]
http2: true
HTTP/1.1 405
