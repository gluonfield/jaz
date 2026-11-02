// Signs the notch helper as Jaz itself: macOS answers its notification
// settings reads only for code signed under the bundle identifier it claims.
import { sign } from 'app-builder-lib/out/codeSign/macCodeSign.js'

export default (options, packager) => sign({
  ...options,
  optionsForFile: (file) => {
    const fileOptions = options.optionsForFile(file)
    return file.endsWith('/Contents/MacOS/jaz-notch')
      ? { ...fileOptions, additionalArguments: [...fileOptions.additionalArguments, '--identifier', packager.appInfo.id] }
      : fileOptions
  },
})
