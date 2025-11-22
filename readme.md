This is a pomodoro app using Go fyne.

[Mac(Apple Silicon)](https://github.com/refirst11/pompom/releases/latest/download/pompom.arm64.zip) -
[Mac(Intel)](https://github.com/refirst11/pompom/releases/latest/download/pompom.x64.zip) - [Linux](https://github.com/refirst11/pompom/releases/latest/download/pompom.linux.zip) -
[Windows](https://github.com/refirst11/pompom/releases/latest/download/pompom.exe.zip)

<img width="398" height="519" alt="2025-11-22" src="https://github.com/user-attachments/assets/5dc58f58-8b2e-4b69-94ff-57eba0d659f5" />

## Downloads

Download the zip file and unzip it to create the application file.

On a Mac, move the app to Applications and then launch it using the following command for shell.

```sh
xattr -cr /Applications/pompom.app
```

## Local build

After cloning, run the following in the project.

app run for command `go run main.go`.  
create the application files.

```sh
env GOFLAGS="-buildvcs=false" fyne package -os darwin -icon icon.png
```
