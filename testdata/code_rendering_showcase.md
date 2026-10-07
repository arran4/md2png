# Code Rendering Showcase

## Literal tabs and mixed spacing

```go
func main() {
	// A literal tab indentation
	println("Hello")
    // Mixed spaces and tabs
	println("Mixed")
	println("Tabs	after	text")
		println("Multiple		tabs")
}
```

## Plain Code Block

```
Here is a plain block
	With a leading tab
    And leading spaces
Blank line below:

	Another tab here
```

## Indented Code Block

    func main() {
	// This is indented code containing a tab
	println("Indented")
    }

## Nested List Fenced Code

* Item 1
  * Item 2
    ```go
    func test() {
	// Nested code block with tab
	return
    }
    ```

## Representative Punctuation and UTF-8

```
{} [] () <> " ' \ / | & # % @ ~ ^ _ - + = : ; , . ? ! $ *
Special controls and spaces:
NBSP ( ) ThinSpace ( ) EmSpace ( )
```

## Printable UTF-8 set
```go
// Representative printable UTF-8 (ASCII letters/digits omitted, testing accents & symbols)
const café = "café"
const naïve = "naïve"
const micro = "µ"
const omega = "Ω"
```
