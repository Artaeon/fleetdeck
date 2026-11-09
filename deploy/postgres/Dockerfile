# Optional PostgreSQL 17 image for explicitly non-root deployments.
# Review and scan a new upstream digest before updating this pin.
FROM postgres:17-alpine@sha256:3540c0362c809df084ffac0ae69fe794515f9de92692f02178bcdce0ef0302ac
USER root
# The upstream entrypoint uses gosu only when started as root. This image always
# starts as postgres, so the unused privilege-switching binary is unnecessary.
RUN rm /usr/local/bin/gosu
USER 70:70
