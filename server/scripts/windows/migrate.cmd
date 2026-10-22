@echo off
setlocal

set "MIGRATE_COMMAND=%~1"
if "%MIGRATE_COMMAND%"=="" set "MIGRATE_COMMAND=up"
set "MIGRATE_NAME=%~2"

if /I "%MIGRATE_COMMAND%"=="up" goto run
if /I "%MIGRATE_COMMAND%"=="up-by-one" goto run
if /I "%MIGRATE_COMMAND%"=="down" goto run
if /I "%MIGRATE_COMMAND%"=="version" goto run

if /I "%MIGRATE_COMMAND%"=="status" goto run
if /I "%MIGRATE_COMMAND%"=="new" goto new

echo Usage: migrate.cmd ^<up^|up-by-one^|down^|version^|status^|new^> [name]
exit /b 2

:new
if "%MIGRATE_NAME%"=="" (
    echo Usage: migrate.cmd new ^<name^>
    exit /b 2
)

:run
pushd "%~dp0..\.."
go run ./cmd/migrate %MIGRATE_COMMAND% %MIGRATE_NAME%
set "MIGRATE_EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %MIGRATE_EXIT_CODE%
