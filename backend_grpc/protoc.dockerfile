FROM bufbuild/buf:latest

WORKDIR /app

ENTRYPOINT ["buf", "generate"]