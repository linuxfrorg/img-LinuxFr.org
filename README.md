External images on LinuxFr.org
==============================

Our users can use images from external domains on LinuxFr.org.
This component is a reverse-proxy / cache for these images.

The main benefits of using a proxy instead of linking directly the images are:

- **No flood**: images can be hosted on small servers that are not able to
  handle all the traffic from LinuxFr.org, so we avoid to flood them
- **History**: even if a server is taken down, we are able to keep serving
  images that are already used on our pages
- **Security**: on the HTTPS pages, we won't include images from other domains
  that are available only in HTTP, so it prevents browsers from displaying
  warning about unsafe pages
- **Privacy**: the users won't connect to the external domains, so their IP
  addresses won't be logged on these servers.

Side effects:

- during refresh period, if the image is changed on remote side (modified or converted into another format), the new image will be served after the next fetch;
- after refresh period, the image is served from the cache;
- if the image is deleted on remote side, the image is served from the cache.

How to use it? (outside Docker)
-------------------------------

[Install Go](http://golang.org/doc/install) and don't forget to set `$GOPATH`

    $ go get -v -u github.com/linuxfrorg/img-LinuxFr.org
    $ img-LinuxFr.org [-a addr] [-c] [-d dir] [-e avatar] [-l log] [-p refresh] [-r redis] [-u agent]

And, to display the help:

    $ img-LinuxFr.org -h

How to use it? (with Docker)
-------------------------------

Build and run Docker image:

    $ docker build --tag linuxfr.org-img .
    $ docker run --publish 8000:8000 linuxfr.org-img

How it works?
-------------

Accepted requests are:
- `GET /status` (expected answer is HTTP 200 with "OK" body)
- `GET /img/<encoded_uri>` or `GET /img/<encoded_uri>/<filename>`
- `GET /avatars/<encoded_uri>` or `GET /avatars/<encoded_uri>/<filename>`

where `<filename>` is the name given to the file, and `encoded_uri` is the `uri` converted into hexadecimal string.

Example: `http://nginx/red_100x100.png` could be accessed as `GET /img/687474703A2F2F6E67696E782F7265645F313030783130302E706E67/square_red.png`

```mermaid
graph TD
  A[ HTTP request ] --> B[ Status /status ]

  B --> |GET| SA[ 200 ]
  B --> |otherwise| SB[ 405 ]

  A --> C[ Avatar /avatars/ or image /img/ ]
  C --> AA[ bad or invalid path or method 40x]
  C --> AC[ check url status]
  AC --> AD[ undeclared image or invalid URI or admin block 404]
  AC --> AG[ already in cache]
  AC --> AH[ previous fetch in error]
  AH --> | in cache| AK[ serve from cache]
  AH --> | not in cache | AP[ answers 404]
  AG --> AK
  AC --> AI[ fetch during refresh period]
  AI --> | any DNS/TLS/HTTP error | AM[ answers 404]
  AI --> | not a 200/304 or too big content or bad content-type | AN[ set in error]
  AN --> AM
  AI --> AO[manipulate aka resize if avatar]
  AO --> AK
```

- HTTP 404s for avatars are converted into redirection to default avatar address.
- `declared` means that `img/<uri>` in Redis contains a `created_at` field.
- `admin block` means that `img/<uri>` in Redis contains a `status` field with "Blocked" value.
- `in error` means that `img/err/<uri>` in Redis exists and file is not in cache from a previous fetch.
- `in cache` means that `img/<uri>` in Redis contains a `checksum` field. And if `img/updated/<uri>` exists, the cache is up-to-date with the remote server (last update less than one hour). And if `img/updated/<uri>` doesn't exist, but `created_at` field from `img/<uri>` is older than refresh period, no more update so serve from cache.

```mermaid
graph TD
  A[ undeclared ] -->|img/uri created_at| B[declared]
  B --> |img/uri status Blocked| C[ admin block]
  B --> |img/err/uri| K[ fetch in error]
  K --> |img/uri/checksum| E[ serve from cache disk]
  K --> |not in cache| D[ in error ]
  D --> |cache refresh interval one hour| B
  B --> |img/updated/uri exists or outside refresh period| E
  B --> |no img/updated/uri and during refresh period| F[fetch from server]
  F --> |got 304| G[reset cache timer]
  F --> |got 200| H[save in cache]  
  F --> K
  F --> |img/err/uri| D
  H --> |different checksum| I[save on disk]
  H --> |same checksum| G
  I --> |img/uri type, checksum, etag| J[on disk]
  J --> G
  G --> |cache refresh interval| B
```

Redis schema
------------
(extracted from [full LinuxFr.org Redis schema](https://github.com/linuxfrorg/linuxfr.org/blob/main/db/redis.txt))

Key                                            | Type   | Value                 | Expiration | Description
---------------------------------------------- | ------ | --------------------- | ---------- | -------------------
`img/<uri>`                                    |  hash  |                       |     no     | Images, with fields 'created_at': seconds since Epoch, 'status': 'Blocked' if administratively blocked (by moderation), 'type': content-type like 'image/jpeg' (set by `img` daemon), 'checksum': SHA1 (set by `img` daemon), and 'etag': etag (set by `img` daemon)
`img/blocked`                                  |  list  |         URIs          |     no     | Images blocked by moderation team
`img/err/<uri>`                                | string |         error         |     1h     | Image fetch in error, like "Invalid content-type", created by `img` daemon
`img/latest`                                   |  list  |         URIs          | no, limited| Last images as `<uri>`, limited to NB_IMG_IN_LATEST = 100
`img/updated/<uri>`                            | string |        modtime        |     1h     | Cached images, created by `img` daemon, value like "Thu, 12 Dec 2013 12:28:47 GMT"

Testsuite
---------
Testsuite requires docker-compose.

```bash
cd tests/
docker-compose up --build
```

If everything went well, expect at the end:

```
linuxfr.org-img-test_1  | All tests looks good!
tests_linuxfr.org-img-test_1 exited with code 0
```

Extra checks
------------

Linter for Dockerfile:

```bash
for image in Dockerfile tests/Dockerfile
do
  # Test with pinned hadolint/hadolint:v2.15.1-debian
  docker run --rm --interactive hadolint/hadolint@sha256:9a3944b7fddcb947d1ffd90829ac1a6e5c30479223358f249d8b96c7d0019e27 < "$image"
  # Test with replicated/dockerfilelint but last push more than 5 years ago...
  # docker run --rm --volume $(pwd)/$image:/app/Dockerfile --workdir /app replicated/dockerfilelint@sha256:15ce784e5847966b6d9a88cba348a9429f8b5212f6017180f10ce36b472dfe52 Dockerfile
done
```

Linter for Go:

```bash
docker run --rm --tty --volume $(pwd):/app --workdir /app golangci/golangci-lint:v2.13.2 golangci-lint run --verbose
```

Vulnerability/secret scanners:

```bash
docker run --rm --volume $(pwd):/app --workdir /app aquasec/trivy:0.74.0 repo .
docker run --rm --volume $(pwd):/app --workdir /app chainguard/grype:latest --name linuxfr.org-img --verbose dir:/app
```

See also
--------

* [Git repository for img-LinuxFr.org](https://github.com/linuxfrorg/img-LinuxFr.org)

Copyright
---------

The code is licensed as GNU AGPLv3. See the LICENSE file for the full license.

♡2012-2018 by Bruno Michel. Copying is an act of love. Please copy and share.

2022-2026 by Benoît Sibaud and Adrien Dorsaz.
