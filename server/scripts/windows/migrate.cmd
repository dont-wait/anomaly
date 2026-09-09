@echo off
setlocal

set "MIGRATE_COMMAND=%~1"
if "%MIGRATE_COMMAND%"=="" set "MIGRATE_COMMAND=up"

if /I "%MIGRATE_COMMAND%"=="up" goto run
if /I "%MIGRATE_COMMAND%"=="up-by-one" goto run
if /I "%MIGRATE_COMMAND%"=="down" goto run
if /I "%MIGRATE_COMMAND%"=="version" goto run

echo Usage: migrate.cmd ^<up^|up-by-one^|down^|version^>
exit /b 2

:run
pushd "%~dp0..\.."
go run ./cmd/migrate %MIGRATE_COMMAND%
set "MIGRATE_EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %MIGRATE_EXIT_CODE%
