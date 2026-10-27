# Supply a reviewed immutable local image or registry digest, never a moving tag.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
USER root
# Only suitable for entrypoints, migrations and jobs invoking Node directly.
RUN command -v node \
    && rm -rf /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/corepack /opt/yarn-* \
    && rm -f /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack \
      /usr/local/bin/yarn /usr/local/bin/yarnpkg /usr/local/bin/pnpm /usr/local/bin/pnpx
ARG RUNTIME_UID
ARG RUNTIME_GID
RUN test "$RUNTIME_UID" -gt 0 && test "$RUNTIME_GID" -gt 0
USER ${RUNTIME_UID}:${RUNTIME_GID}
