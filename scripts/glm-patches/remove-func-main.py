#!/usr/bin/env python3
"""
Remove func main() from a Go file, handling nested braces correctly.

Usage: remove-func-main.py <file.go>

This is more robust than awk/sed because it counts braces to find the
end of the function, even if it contains nested blocks (if, for, etc.).
"""
import sys

def remove_func_main(content):
    lines = content.split('\n')
    result = []
    i = 0
    while i < len(lines):
        line = lines[i]
        # Match "func main() {" at start of line (possibly with leading whitespace)
        if line.lstrip().startswith('func main()'):
            # Skip this entire function
            # Count braces to find the end
            brace_count = 0
            started = False
            while i < len(lines):
                for ch in lines[i]:
                    if ch == '{':
                        brace_count += 1
                        started = True
                    elif ch == '}':
                        brace_count -= 1
                if started and brace_count == 0:
                    i += 1
                    break
                i += 1
            continue
        result.append(line)
        i += 1
    return '\n'.join(result)

def main():
    if len(sys.argv) != 2:
        print("Usage: remove-func-main.py <file.go>", file=sys.stderr)
        sys.exit(1)
    
    path = sys.argv[1]
    with open(path, 'r') as f:
        content = f.read()
    
    if 'func main()' not in content:
        print(f"  No func main() found in {path}")
        return
    
    new_content = remove_func_main(content)
    
    with open(path, 'w') as f:
        f.write(new_content)
    
    print(f"  Removed func main() from {path}")

if __name__ == '__main__':
    main()
