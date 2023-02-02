# Moe-Sticker-Bot Import Component

## Description
This package is intended to fetch, parse, download and convert LINE and KakaoTalk Stickers from share link.

It is designed to be able to operate independentaly from moe-sticker-bot core so third party apps can also utilize this package.

## Usage

A typical workflow is to call `parseImportLink` then `prepareImportStickers`.

```go
import "github.com/star-39/moe-sticker-bot/pkg/msbimport"

ctx, cancel := context.WithCancel(context.Background())
ld := &msbimport.LineData{}

//LineData will be parsed to ld.
warn, err := msbimport.ParseLineLink("https://store.line.me/stickershop/product/27286", ld)
if err != nil {
    //handle error here.
}
if warn != nil {
    //handle warning message here.
}

err = msbimport.PrepareImportStickers(ctx, ld, "./", false)
if err != nil {
    //handle error here.
}

for i, lf := range ld.Files {
    lf.Wg.Wait()
    if lf.CError != nil {
        //hanlde sticker error here.
    }
    println(lf.OriginalFile)
    println(lf.ConvertedFile)
    //...
}

//Your stickers files will appear in the work dir you specified.
```

## License
GPL v3 License.

Source code of this package MUST ALWAYS be disclosed no matter what use case is, 

and source code referring to this package MUST ALSO be discolsed and share the same GPL v3 License.