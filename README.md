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

- file is changed on remote side (modified or converted into another format), new file will be served after the next fetch
- file is deleted on remote side, file won't be served after the next try to fetch

How to use it? (outside Docker)
-------------------------------

[Install Go](http://golang.org/doc/install) and don't forget to set `$GOPATH`

    $ go get -v -u github.com/linuxfrorg/img-LinuxFr.org
    $ img-LinuxFr.org [-a addr] [-r redis] [-l log] [-d dir]

And, to display the help:

    $ img-LinuxFr.org -h

How to use it? (with Docker)
-------------------------------

Build and run Docker image:

    $ docker build -t linuxfr.org-img .
    $ docker run --publish 8000:8000 linuxfr.org-img
    or
    $ docker run --publish 8000:8000 --env REDIS=someredis:6379/1 linuxfr.org-img

Why don't you use camo?
-----------------------

(answer from Bruno Michel, in 2012)

Github has developed a similar service, camo, for their own usage.
I used it as a source of inspiration but I prefered to redevelop a new service
instead of using it for several reasons:

- It lacks some feature and particulary caching!
- It runs with a legacy version of node.js, which is not very friendly for our
  sysadmins.
- I plan to extend it for other usages in the future, and I prefer coding in
  golang than in nodejs.
- And it's a fun component to recode :p

Redis schema
------------
(extracted from [full LinuxFr.org Redis schema](https://github.com/linuxfrorg/linuxfr.org/blob/master/db/redis.txt))

Key                                            | Type   | Value                 | Expiration | Description
-----------------------------------------------|--------|---------------------------------------------------------
`img/<uri>`                                    |  hash  |                       |     no     | Images, with fields 'created_at': seconds since Epoch, 'status': 'Blocked' if administratively blocked (by moderation), 'type': content-type like 'image/jpeg' (set by `img` daemon), 'checksum': SHA1 (set by `img` daemon), and 'etag': etag (set by `img` daemon)
`img/err/<uri>`                                | string |         error         |     no     | Images in error, like "Invalid content-type", created by `img` daemon but removed by `dlfp`
`img/updated/<uri>`                            | string |        modtime        |     1h     | Cached images, created by `img` daemon, value like "Thu, 12 Dec 2013 12:28:47 GMT"

Testsuite
---------
Testsuite requires [Hurl](https://hurl.dev/) and docker-compose.

```
cd tests/
docker-compose up --build
DEBUG=1 ./img-tests.sh
```

See also
--------

* [Git repository](https://github.com/linuxfrorg/img-LinuxFr.org)
* [Camo](https://github.com/atmos/camo)

Copyright
---------

The code is licensed as GNU AGPLv3. See the LICENSE file for the full license.

♡2012-2018 by Bruno Michel. Copying is an act of love. Please copy and share.
2022-2024 by Benoît Sibaud and Adrien Dorsaz.
