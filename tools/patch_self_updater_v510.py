from pathlib import Path

p = Path('buildsrc/main.go')
text = p.read_text()

main_marker = 'func main() {'
if main_marker not in text:
    raise SystemExit('main function not found')
text = text.replace(
    main_marker,
    main_marker + '\n\t// A healthy v5.1+ Supervisor also serves as its own zero-UI companion updater.\n\tif pcOSMaybeRunUpdaterMode() {\n\t\treturn\n\t}',
    1,
)

cloud_marker = '\tgo pcOSCloudLoop()'
if cloud_marker not in text:
    raise SystemExit('pcOSCloudLoop startup marker not found')
text = text.replace(
    cloud_marker,
    cloud_marker + '\n\tgo pcOSCompanionUpdaterLoop()',
    1,
)

p.write_text(text)
