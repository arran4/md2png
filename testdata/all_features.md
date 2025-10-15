# Rendering Showcase

Paragraph one demonstrates the new paragraph spacing. It should feel like there is extra breathing room before the next paragraph.

Paragraph two follows immediately after the first to highlight the 1.5 line break spacing requirement.

## Lists

- Top level bullet
  - Nested bullet one
    - Nested bullet two
- Second bullet with more text to ensure wrapping behaviour is visible when the content flows onto another line within the same list item.

1. First ordered item
2. Second ordered item
   1. Nested ordered item
   2. Another nested ordered item with enough text to wrap around onto the next line and show numbering alignment.
3. Third ordered item after the nested list.

## Code

```
    def example(value):
        print("value:", value)
        return value * 2
```

## Links

Here is a [link that should look blue and underlined](https://example.com) inside the text.

## Table

| Feature | Status |
| :------ | :----- |
| Tables  | Rendered |
| Links   | Blue and underlined |
| Lists   | Support nesting |

## Unsupported Syntax

::: custom-block
This block should display a warning indicator because the renderer does not support the `:::` syntax yet.
:::
