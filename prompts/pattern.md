# Conway's Game of Life Pattern Addition Instructions

**CRITICAL: Follow these instructions precisely. Pattern errors break the game.**

## Quick Start Workflow

For each pattern URL provided:

1. **EXTRACT** → Read website (use raw HTML), find pattern data (O = alive, . = dead)
2. **POSITION** → Find correct alphabetical location in `internal/patterns/patterns.go` Patterns object
3. **ADD** → Insert pattern using exact format below
4. **VERIFY** → Check pattern data matches original exactly

## File Location

- **Target**: `internal/patterns/patterns.go`
- **Object**: `Patterns`
- **Function**: `init`
- **Order**: Alphanumeric (numbers before letters)

## Pattern Format (MUST FOLLOW)

### Direct Copy Process

```
Extract from website → Copy exactly with O's and .'s → Paste into template
```

**Pattern Format:**

- **O** = alive cell
- **.** = dead cell  
- **|** = row delimiters (surround each row)

**Example:**

```
|...O.O...|
|.O.....O.|
|..OOOOO..|
```

## Pattern Template

```go
"pattern-key": parsePattern("Pattern Name", `
    |row data here|
    |each row in pipes|
`),
```

## Ordering Examples

- '119P4H1V0' (numbers first)
- 'acorn' (letters after)
- 'bi-gun' (alphabetical)

## Verification Checklist

✅ Original pattern copied exactly with O's and .'s  
✅ Pattern dimensions unchanged  
✅ Correct alphabetical placement  
✅ Each row surrounded by | delimiters

## Common Fatal Errors

❌ Converting O's or .'s to other characters  
❌ Wrong alphabetical placement  
❌ Changing pattern dimensions  
❌ Missing | delimiters around rows
