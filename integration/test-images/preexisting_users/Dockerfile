FROM busybox:latest

RUN adduser -D alice && \
  adduser -D bob

USER alice
RUN echo "hi from Alice" > /home/alice/alicesfile

USER bob
RUN echo "hi from Bob" > /home/bob/bobsfile
