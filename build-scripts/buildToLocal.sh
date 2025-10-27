#! /bin/bash

# If MacOS

if [[ "$OSTYPE" == "darwin"* ]]; then
    sudo go build -o /usr/local/bin/mastercrab
fi
