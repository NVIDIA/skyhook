#!/usr/bin/env python3

# 
# LICENSE START
#
#    Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.
#
# LICENSE END
# 

import os
import argparse
import re
from typing import Dict, List, Tuple
import fnmatch

# Comment style definitions for different file types with regex patterns
COMMENT_STYLES = {
    r'.*\.py$|.*\.sh$|.*\.ya?ml$|.*\.Dockerfile$|^Dockerfile$': {
        'start': '# ',
        'line': '# ',
        'end': '# '
    },
    r'.*\.go$': {
        'start': '/*',
        'line': ' * ',
        'end': ' */'
    },
}

# Built-in ignore patterns
BUILT_IN_IGNORE_PATTERNS = [
    # Vendor directories
    'vendor',
    'vendor/*',
    # Go vendor directories
    'Godeps',
    'Godeps/*',
    # Node.js vendor directories
    'node_modules',
    'node_modules/*',
    # Python vendor directories
    'venv',
    'venv/*',
    '.env',
    '.env/*',
    'env',
    'env/*',
    # Chart directory
    'chart',
    'chart/*'
]

def should_ignore(path: str, ignore_patterns: List[str]) -> bool:
    """Check if a path should be ignored based on ignore patterns."""
    path_parts = path.split(os.sep)
    
    for pattern in ignore_patterns:
        # Check if the pattern matches the full path
        if fnmatch.fnmatch(path, pattern):
            return True
        
        # Check if pattern matches any part of the path
        for part in path_parts:
            if fnmatch.fnmatch(part, pattern):
                return True
            
    return False

def read_license_template(template_path: str) -> str:
    """Read the license template file."""
    with open(template_path, 'r') as f:
        return f.read().strip()

def format_license(license_text: str, comment_style: Dict[str, str]) -> str:
    """Format the license text with the appropriate comment style."""
    lines = license_text.split('\n')
    formatted = [
        comment_style['start'],
        f"{comment_style['line']}LICENSE START",
        comment_style['line'].rstrip()
    ]
    
    for line in lines:
        if line.strip():
            formatted.append(f"{comment_style['line']}{line}")
        else:
            formatted.append(f"{comment_style['line'].rstrip()}")
    
    formatted.extend([
        comment_style['line'].rstrip(),
        f"{comment_style['line']}LICENSE END",
        comment_style['end']
    ])
    return '\n'.join(formatted)

def find_files(root_dir: str, patterns: List[str], ignore_patterns: List[str]) -> List[str]:
    """Find all files matching the regex patterns recursively, respecting ignore patterns."""
    matches = []
    for root, _, filenames in os.walk(root_dir):
        # Get relative path from root_dir
        rel_root = os.path.relpath(root, root_dir)
        
        # Skip if this directory should be ignored
        if should_ignore(rel_root, ignore_patterns):
            continue
            
        for filename in filenames:
            rel_path = os.path.join(rel_root, filename)
            
            # Skip if the file should be ignored
            if should_ignore(rel_path, ignore_patterns):
                continue
                
            # Check if the file matches any of our patterns
            for pattern in patterns:
                if re.match(pattern, filename):
                    matches.append(os.path.join(root, filename))
                    break  # No need to check other patterns once we have a match
                    
    return matches

def find_existing_license(content: str) -> Tuple[int, int]:
    """Find the start and end positions of an existing license header."""
    lines = content.split('\n')
    start_line = 0
    end_line = 0
    
    # Look for LICENSE START and LICENSE END markers
    for i, line in enumerate(lines):
        if 'LICENSE START' in line:
            start_line = i - 1  # Include the opening comment line
        elif 'LICENSE END' in line:
            end_line = i + 2  # Include the closing comment line
            break
    
    return start_line, end_line

def insert_license(file_path: str, formatted_license: str, verbose: bool = False) -> None:
    """Insert the formatted license at the beginning of the file."""
    with open(file_path, 'r') as f:
        content = f.read()
    
    # Find any existing license header
    start_line, end_line = find_existing_license(content)
    
    lines = content.split('\n')
    if start_line != -1 and end_line != -1:
        # Check if the found license is the same as the formatted license
        existing_license = "\n".join(lines[start_line:end_line])
        if existing_license == formatted_license:
            if verbose:
                print(f"License is already formatted in {file_path}")
            return

        # Remove existing license
        print(f"Replacing existing license in {file_path}")
        lines = lines[:start_line] + lines[end_line:]
        content = '\n'.join(lines)
    
    # Strip any leading/trailing whitespace from content
    content = content.strip()
    
    # For Python/Shell files, preserve any shebang line
    if file_path.endswith(('.py', '.sh')):
        lines = content.split('\n')
        if lines and lines[0].startswith('#!'):
            content = lines[0] + '\n\n' + formatted_license + '\n\n' + '\n'.join(lines[1:])
        else:
            content = formatted_license + '\n\n' + content
    else:
        content = formatted_license + '\n\n' + content
    
    # Ensure file ends with exactly one newline
    content = content.rstrip('\n') + '\n'
    
    with open(file_path, 'w') as f:
        f.write(content)
    print(f"Updated license in {file_path}")

def main():
    """License Header Formatting Tool for Multiple File Types.

    This script formats and applies NVIDIA's Apache 2.0 license headers to source code files.
    It supports multiple file types and handles each with appropriate comment styles.
    The script will:

    1. Add license headers to files that don't have them
    2. Replace existing license headers with the standardized format
    3. Preserve shebang lines in scripts
    4. Skip vendor directories and files matching ignore patterns
    5. Add LICENSE START/END markers for easier detection and replacement

    Supported file types:
    - Python (.py)       : Uses # comments
    - Shell (.sh)        : Uses # comments
    - Go (.go)           : Uses /* */ block comments
    - YAML (.yml/.yaml)  : Uses # comments
    - Dockerfile         : Uses # comments (includes both "Dockerfile" and files ending in ".Dockerfile")

    Usage:
        ./format_license.py [--license-file PATH] [--root-dir PATH] [--verbose]

    Arguments:
        --license-file : Path to the Apache 2.0 license file (default: LICENSE)
        --root-dir     : Root directory to search for files (default: current directory)
        --verbose      : Show detailed messages, including when licenses are already formatted

    Example:
        # Format all supported files in the current directory
        ./format_license.py

        # Format files using a specific license file and directory with verbose output
        ./format_license.py --license-file /path/to/LICENSE --root-dir /path/to/project --verbose

    Note:
        The script automatically ignores common vendor directories.
        The chart/ directory is also ignored by default. See BUILT_IN_IGNORE_PATTERNS for more details.
    """
    parser = argparse.ArgumentParser(description='Format and apply license headers to source files')
    parser.add_argument('--license-file', default='LICENSE',  help='Path to the license template file')
    parser.add_argument('--root-dir', default='.',  help='Root directory to search for files')
    parser.add_argument('--verbose', action='store_true', help='Show detailed messages, including when licenses are already formatted')
    args = parser.parse_args()

    # Read the license template
    license_text = read_license_template(args.license_file)
    
    # Get the boilerplate section from the license
    start_marker = "   APPENDIX: How to apply the Apache License to your work."
    end_marker = "   limitations under the License."
    
    start_idx = license_text.find(start_marker)
    if start_idx != -1:
        # Skip to the actual boilerplate
        boilerplate_start = license_text.find("   Copyright", start_idx)
        if boilerplate_start != -1:
            end_idx = license_text.find(end_marker, boilerplate_start)
            if end_idx != -1:
                license_text = license_text[boilerplate_start:end_idx + len(end_marker)]

    # Read .gitignore patterns
    ignore_patterns = BUILT_IN_IGNORE_PATTERNS

    # Process each file pattern
    for pattern, comment_style in COMMENT_STYLES.items():
        # Format the license for this file type
        formatted_license = format_license(license_text, comment_style)
        
        # Find and process all files matching this pattern
        files = find_files(args.root_dir, [pattern], ignore_patterns)
        for file_path in files:
            insert_license(file_path, formatted_license, args.verbose)

if __name__ == '__main__':
    main() 