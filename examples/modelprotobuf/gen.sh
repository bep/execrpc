#!/bin/bash

protoc --go_out=. --go_opt=module=github.com/bep/execrpc/examples/modelprotobuf model.proto