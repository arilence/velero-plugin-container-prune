# Copyright 2017, 2019, 2020 the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.20-bullseye AS development
ENV GOPROXY=https://proxy.golang.org
RUN useradd --system --create-home --no-log-init --shell /bin/bash abc
# USER abc
# # Install development specific packages
# RUN go install -v github.com/ramya-rao-a/go-outline@v0.0.0-20210608161538-9736a4bde949
# RUN go install -v golang.org/x/tools/gopls@latest

FROM golang:1.20-bullseye AS build
ENV GOPROXY=https://proxy.golang.org
WORKDIR /go/src/github.com/arilence/velero-plugin-container-prune
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/velero-plugin-container-prune .

FROM busybox:1.33.1 AS busybox

FROM scratch
COPY --from=build /go/bin/velero-plugin-container-prune /plugins/
COPY --from=busybox /bin/cp /bin/cp
USER 65532:65532
ENTRYPOINT ["cp", "/plugins/velero-plugin-container-prune", "/target/."]
