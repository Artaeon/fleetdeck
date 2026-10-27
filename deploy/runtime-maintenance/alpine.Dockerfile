# Review the configured Alpine repositories and candidate scan before rollout.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
USER root
# Keep the configured distribution branch; do not add edge/testing repositories.
RUN apk --no-cache upgrade brotli-libs curl libcurl
ARG RUNTIME_UID
ARG RUNTIME_GID
RUN test "$RUNTIME_UID" -gt 0 && test "$RUNTIME_GID" -gt 0
USER ${RUNTIME_UID}:${RUNTIME_GID}
