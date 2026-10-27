# Optional PostgreSQL 17 image for explicitly non-root deployments.
# Review and scan a new upstream digest before updating this pin.
FROM postgres:17-alpine@sha256:266d3c257594e195b309ae9d1805e54c788f1d7b375f56bf6c367aac6c917e7b
USER root
# The upstream entrypoint uses gosu only when started as root. This image always
# starts as postgres, so the unused privilege-switching binary is unnecessary.
RUN rm /usr/local/bin/gosu
USER 70:70
