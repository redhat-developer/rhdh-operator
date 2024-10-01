FROM scratch

# Copy files to locations specified by labels.
COPY manifests /manifests/
COPY metadata /metadata/

# append Brew metadata here
