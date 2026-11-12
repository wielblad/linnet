#!/bin/sh
# Takes one screenshot. Usage: sh shots.sh <out-name> [theme] [shot2 args...]
ROOT='C:\Users\wiela\AppData\Roaming\Claude\scratch-workspaces\2dc58228-56b0-4014-81e4-b8af904f1fdf\3cf92da4-5975-4251-a651-72d8757c4316\scratch-2026-11-12-283f71'
SP='C:\Users\wiela\AppData\Local\Temp\claude\1ece9906cc35470d\245ee484-d02e-40c7-9690-1377387871e7\scratchpad'
name="$1"; theme="$2"; shift; shift
MLITE_THEME="$theme" powershell -NoProfile -ExecutionPolicy Bypass -File "$SP\\shot2.ps1" -Exe "$ROOT\\mlite\\build\\mlite.exe" -DataDir "$ROOT\\build\\data-ui-test" -Out "$SP\\$name.png" "$@" 2>&1 | tail -3
