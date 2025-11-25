# LSP Typo Detection and Syntax Validation

## New Features Added

### 1. Nested Struct Type Detection
The LSP now properly detects and validates nested struct types (variant types).

**Example:**
```ahoy
struct person:
    name: string
    age: int
    type dwarf:
        height: int
    type giant:
        height: int
$

person2: dwaf{name: "Alice", age: 30, height: 12}  
? Error: Struct type 'dwaf' does not exist, did you mean 'person:dwarf'?

person3: peeson{name: "Bob", age: 25}
? Error: Struct type 'peeson' does not exist, did you mean 'person'?
```

**Implementation:**
- `type_typos.go`: Enhanced `collectTypes()` to track nested type declarations
- Valid types now include both `dwarf` and `person:dwarf` formats
- Uses Levenshtein distance algorithm to suggest closest matches

### 2. Object Property Typo Detection
The LSP now validates property names in object literals against struct definitions.

**Example:**
```ahoy
person2: dwarf{nae: "Alice", ag: 30, height: 12}
? Error: Property 'nae' does not exist on struct 'dwarf', did you mean 'name'?
? Error: Property 'ag' does not exist on struct 'dwarf', did you mean 'age'?
```

**Implementation:**
- `type_typos.go`: Added `checkObjectProperties()` function
- Validates properties against both Ahoy structs and C header structs (e.g., raylib types)
- Provides suggestions for misspelled property names

### 3. Object Access Syntax Validation
The LSP now detects when the wrong access syntax is used for different data types.

**Example:**
```ahoy
object_numbers: object = {one:"one", two:"two", three:"three"}
first: object_numbers["one"]
? Error: Invalid object access syntax, use object{} instead of array[]
```

**Correct Access Syntax:**
- **Arrays**: `arr[index]`
- **Dicts**: `dict<key>`
- **Objects/Structs**: `obj{property}`

**Implementation:**
- `access_syntax.go`: New file implementing `checkAccessSyntax()`
- Checks `NODE_ARRAY_ACCESS` nodes against variable types
- Detects when `[]` syntax is used on struct types

## Technical Details

### Levenshtein Distance Algorithm
Used to find the closest matching identifier:
- Maximum distance threshold: 3 edits
- Handles insertions, deletions, and substitutions
- Provides "did you mean" suggestions

### C Header Integration
All validations work with imported C types:
```ahoy
import "/path/to/raylib.h"

rect: Rectangl{x: 0, y: 0, width: 100, height: 50}
? Error: Struct type 'Rectangl' does not exist, did you mean 'Rectangle'?

rect2: Rectangle{x: 0, y: 0, wdth: 100, height: 50}
? Error: Property 'wdth' does not exist on struct 'Rectangle', did you mean 'width'?
```

## Files Modified/Created

### New Files:
- `access_syntax.go` - Object access syntax validation

### Modified Files:
- `type_typos.go`:
  - Enhanced nested type collection
  - Added property validation
  - Added `checkObjectProperties()` function
  - Added `findClosestProperty()` function
- `diagnostics.go`:
  - Added `checkAccessSyntax()` call to diagnostic pipeline
- `server.go`:
  - Error handling improvements (from previous fix)

## Build & Install
```bash
cd /home/lee/Documents/ahoy-lang/ahoy-lsp
go build -o ahoy-lsp .
cp ahoy-lsp /home/lee/bin/
```

## Testing
The LSP will now provide helpful error messages for:
1. ✅ Misspelled struct type names (including nested types)
2. ✅ Misspelled property names in object literals
3. ✅ Wrong access syntax ([], {}, <>)
4. ✅ Support for both Ahoy and C types (raylib, etc.)

Restart your editor to use the updated LSP server.
