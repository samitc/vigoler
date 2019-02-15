FROM golang AS builder

WORKDIR /usr/src/app
RUN apt-get update \
    && apt-get install -y --no-install-recommends bzip2 xz-utils
RUN wget https://bitbucket.org/ariya/phantomjs/downloads/phantomjs-2.1.1-linux-x86_64.tar.bz2 && \
    tar -xf phantomjs-2.1.1-linux-x86_64.tar.bz2 && \
    mv phantomjs-2.1.1-linux-x86_64/bin/phantomjs .
RUN wget https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz  && \
    tar -xf ffmpeg-release-amd64-static.tar.xz && \
    find . -name ffmpeg -exec mv {} . \; && \
    find . -name ffprobe -exec mv {} . \;
RUN curl -L https://yt-dl.org/downloads/latest/youtube-dl -o youtube-dl && \
    chmod a+rx youtube-dl
COPY . .
RUN go build -v -o app ./server/

FROM ubuntu
WORKDIR /usr/local/bin
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 curl fontconfig ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/python3 /usr/bin/python
COPY --from=builder /usr/src/app/app /usr/src/app/youtube-dl /usr/src/app/ffmpeg /usr/src/app/ffprobe /usr/src/app/phantomjs ./
CMD ["./app"]
