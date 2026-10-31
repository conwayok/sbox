#!/usr/bin/env bash
rm -rf out/* && dotnet publish Sbox/Sbox.csproj --configuration Release --runtime osx-arm64 --output out
