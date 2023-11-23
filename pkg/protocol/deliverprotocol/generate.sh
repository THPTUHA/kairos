#!/bin/bash

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@latest
# go install github.com/fatih/gomodifytags@v1.13.0
# go install github.com/FZambia/gomodifytype@latest
# go install github.com/mailru/easyjson/easyjson@latest

GOPATH=$(go env GOPATH)

which protoc
which gomodifytype
which gomodifytags
protoc-gen-go --version
which protoc-gen-go-vtproto
which easyjson

protoc --go_out=. --plugin protoc-gen-go=${GOPATH}/bin/protoc-gen-go --go-vtproto_out=. \
  --plugin protoc-gen-go-vtproto=${GOPATH}/bin/protoc-gen-go-vtproto \
  --go-vtproto_opt=features=marshal+unmarshal+size \
  deliver.proto

gomodifytype -file deliver.pb.go -all -w -from "[]byte" -to "Raw"

echo "replacing tags of structs for JSON backwards compatibility..."
gomodifytags -file deliver.pb.go -field User -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field Client -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field Presence -struct PresenceResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field NumClients -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field NumUsers -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field Offset -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field Epoch -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file deliver.pb.go -field Publications -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null

# compile easy json in separate dir since we are using custom writer here.
mkdir build
cp deliver.pb.go build/deliver.pb.go
cp raw.go build/raw.go
cd build
easyjson -all -no_std_marshalers deliver.pb.go
cd ..
# move compiled to current dir.
cp build/deliver.pb_easyjson.go ./deliver.pb_easyjson.go
rm -rf build

# need to replace usage of jwriter.Writer to custom writer.
find . -name 'deliver.pb_easyjson.go' -print0 | xargs -0 sed -i "" "s/jwriter\.W/w/g"
# need to replace usage of jwriter package constants to local writer constants.
find . -name 'deliver.pb_easyjson.go' -print0 | xargs -0 sed -i "" "s/jwriter\.N/n/g"
# cleanup formatting.
goimports -w deliver.pb_easyjson.go
