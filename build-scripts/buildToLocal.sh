#! /bin/bash

# If MacOS

if [[ "$OSTYPE" == "darwin"* ]]; then
     go build -o $HOME/.local/bin/mastercrab
fi
