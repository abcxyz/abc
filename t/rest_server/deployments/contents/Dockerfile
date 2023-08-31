# Copyright 2023 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Use distroless for ca certs.
FROM gcr.io/distroless/static AS distroless
FROM scratch

# Certs needed for https calls not included in scratch
COPY --from=distroless /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ARG APP
COPY $APP /server

# Run the server on container startup.
ENV PORT 8080
EXPOSE 8080
ENTRYPOINT ["/server"]
