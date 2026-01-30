#!/usr/bin/env python3

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


"""
License Header Formatting Tool

This script automatically adds, updates, or validates Apache 2.0 license headers
in source code files. It supports multiple file types and uses appropriate comment
styles for each.

Key Features:
- Detects existing license headers and updates them if needed
- Preserves shebang lines in shell/Python scripts
- Supports multiple file types with appropriate comment styles
- Normalizes license content for accurate comparison
- Ignores vendor directories and other unwanted files
- Uses SPDX headers for modern license identification

Workflow:
1. Scan directories for supported file types
2. For each file, check if it has a license header
3. If no header exists, add one
4. If header exists but is outdated, replace it
5. If header is already correct, skip the file
"""

import os
import argparse
import re
from typing import List, Tuple
import fnmatch
import subprocess

# Comment style definitions for different file types
# Maps regex patterns (for matching file names) to comment prefixes
COMMENT_STYLES = {
    # Hash-style comments: Python, Shell, YAML, Dockerfile
    r'.*\.py$|.*\.sh$|.*\.ya?ml$|.*\.Dockerfile$|^Dockerfile$': '# ',
    # Block-style comments: Go files (inside /* */ block)
    r'.*\.go$': ' * ',
}

# Built-in ignore patterns - directories and files to skip
# These patterns use fnmatch-style wildcards (* and ?)
# The script will skip any file or directory that matches these patterns
BUILT_IN_IGNORE_PATTERNS = [
    # Vendor directories (third-party dependencies)
    'vendor',        # Exact directory name
    'vendor/*',      # Anything inside vendor directory
    
    # Go vendor directories (legacy Go dependency management)
    'Godeps',        # Godeps directory
    'Godeps/*',      # Anything inside Godeps
    
    # Node.js vendor directories
    'node_modules',  # Node.js dependencies
    'node_modules/*',
    
    # Python virtual environment directories
    'venv',          # Virtual environment
    'venv/*',
    '.env',          # Environment files
    '.env/*',
    'env',           # Another common venv name
    'env/*',
    
    # Helm chart directory (has its own license structure)
    'chart',         # Chart directory
    'chart/*'        # Anything inside chart directory
]

def get_changed_files_from_git(root_dir: str) -> List[str]:
    """
    Get list of files that have uncommitted changes in git.

    Returns:
        List of absolute file paths with changes
    """
    try:
        # Get modified and staged files
        result = subprocess.run(
            ['git', 'diff', '--name-only', 'HEAD'],
            capture_output=True,
            text=True,
            cwd=root_dir,
            check=True
        )

        result2 = subprocess.run(
            ['git', 'diff', '--name-only', '--cached'],
            capture_output=True,
            text=True,
            cwd=root_dir,
            check=True
        )

        changed_files = set()
        for line in result.stdout.strip().split('\n'):
            if line:
                changed_files.add(os.path.abspath(os.path.join(root_dir, line)))

        for line in result2.stdout.strip().split('\n'):
            if line:
                changed_files.add(os.path.abspath(os.path.join(root_dir, line)))

        return list(changed_files)

    except (subprocess.CalledProcessError, FileNotFoundError):
        return []

def is_block_comment(comment_prefix: str) -> bool:
    """
    Check if the comment style uses block comments (/* */) vs line comments (#).
    
    Args:
        comment_prefix: The comment prefix string (e.g., '# ' or ' * ')
        
    Returns:
        True if this is a block comment style (like Go), False for line comments
    """
    return comment_prefix == ' * '


def should_ignore(path: str, ignore_patterns: List[str]) -> bool:
    """
    Check if a file path should be ignored based on ignore patterns.
    
    Args:
        path: The file path to check (relative to root directory)
        ignore_patterns: List of fnmatch patterns to match against
        
    Returns:
        True if the path should be ignored, False otherwise
        
    The function checks both the full path and individual path components
    to handle patterns like 'vendor/*' and 'vendor' properly.
    """
    # Split path into components (e.g., "src/vendor/lib.py" -> ["src", "vendor", "lib.py"])
    path_parts = path.split(os.sep)
    
    for pattern in ignore_patterns:
        # Check if the pattern matches the full path
        # Example: pattern "vendor/*" matches "vendor/lib.py"
        if fnmatch.fnmatch(path, pattern):
            return True
        
        # Check if pattern matches any individual part of the path
        # Example: pattern "vendor" matches path "src/vendor/lib.py"
        for part in path_parts:
            if fnmatch.fnmatch(part, pattern):
                return True
            
    return False

def read_license_template(template_path: str) -> str:
    """Read the license template file and extract the boilerplate section.
    
    The Apache 2.0 license file contains the full license text, but we only need
    the boilerplate section that should be included in source files. This function
    extracts that specific section from the APPENDIX.
    """
    with open(template_path, 'r') as f:
        license_text = f.read().strip()
    
    # Extract the boilerplate section from the Apache 2.0 license
    # The boilerplate is located in the APPENDIX section
    start_marker = "   APPENDIX: How to apply the Apache License to your work."
    end_marker = "   limitations under the License."
    
    start_idx = license_text.find(start_marker)
    if start_idx != -1:
        # Find the actual copyright line that starts the boilerplate
        boilerplate_start = license_text.find("   Copyright", start_idx)
        if boilerplate_start != -1:
            # Find the end of the boilerplate section
            end_idx = license_text.find(end_marker, boilerplate_start)
            if end_idx != -1:
                # Extract just the boilerplate text
                return license_text[boilerplate_start:end_idx + len(end_marker)]
    
    # If we can't find the boilerplate section, return the full license text
    # This serves as a fallback in case the LICENSE file format is different
    return license_text

def format_license(license_text: str, comment_prefix: str, year: str = None) -> str:
    """
    Format the license text with appropriate comment style and SPDX headers.
    
    Args:
        license_text: The license boilerplate text from the LICENSE file
        comment_prefix: Comment prefix string for this file type (e.g., '# ' or ' * ')
        year: Copyright year (defaults to current year)
        
    Returns:
        Formatted license header as a string, ready to insert into source files
        
    The function creates a license header with:
    1. SPDX headers for modern license identification
    2. The Apache 2.0 license boilerplate text
    3. Proper comment formatting for the target file type
    """
    import datetime
    
    # Use current year if not specified
    if year is None:
        year = str(datetime.datetime.now().year)
    
    # Determine file type based on comment prefix
    # Go files use block comments /* */, others use line comments
    uses_block_comments = is_block_comment(comment_prefix)
    
    # Build the license header, starting with SPDX headers
    # SPDX headers provide machine-readable license information
    if uses_block_comments:
        # Block comment files (like Go): Start with block comment opener
        formatted = [
            "/*",  # Open block comment
            f" * SPDX-FileCopyrightText: Copyright (c) {year} NVIDIA CORPORATION & AFFILIATES. All rights reserved.",
            f" * SPDX-License-Identifier: Apache-2.0",
            " *"   # Empty line separator
        ]
    else:
        # Line comment files (Python, Shell, YAML, etc.): Use line comments
        formatted = [
            f"{comment_prefix}SPDX-FileCopyrightText: Copyright (c) {year} NVIDIA CORPORATION & AFFILIATES. All rights reserved.",
            f"{comment_prefix}SPDX-License-Identifier: Apache-2.0",
            comment_prefix.rstrip()  # Empty comment line
        ]
    
    # Process the license boilerplate text from the LICENSE file
    lines = license_text.split('\n')
    for line in lines:
        # Clean up the line (remove leading/trailing whitespace)
        cleaned_line = line.strip()
        
        # Skip the original copyright line since we use our own SPDX header
        # This prevents duplication of copyright information
        if cleaned_line.startswith('Copyright (c) NVIDIA CORPORATION'):
            continue
        
        # Add the line with appropriate comment formatting
        if cleaned_line:
            # Non-empty line: add comment prefix + content
            formatted.append(f"{comment_prefix}{cleaned_line}")
        else:
            # Empty line: add just the comment prefix (trimmed)
            formatted.append(f"{comment_prefix.rstrip()}")
    
    # Close the block comment for block comment files
    if uses_block_comments:
        formatted.append(" */")  # Close block comment
    
    # Join all lines with newlines to create the final license header
    return '\n'.join(formatted)

def find_files(root_dir: str, patterns: List[str], ignore_patterns: List[str]) -> List[str]:
    """
    Find all files matching the regex patterns recursively, respecting ignore patterns.
    
    Args:
        root_dir: Root directory to start searching from
        patterns: List of regex patterns to match against filenames
        ignore_patterns: List of fnmatch patterns for files/directories to ignore
        
    Returns:
        List of absolute file paths that match the patterns and aren't ignored
        
    The function walks the directory tree and:
    1. Skips directories that match ignore patterns
    2. Skips files that match ignore patterns
    3. Includes files that match any of the regex patterns
    """
    matches = []
    
    # Walk the directory tree starting from root_dir
    # os.walk yields (root, dirs, files) for each directory
    for root, _, filenames in os.walk(root_dir):
        # Get relative path from root_dir for ignore pattern matching
        # Example: "/path/to/project/src" -> "src"
        rel_root = os.path.relpath(root, root_dir)
        
        # Skip if this directory should be ignored
        # Example: skip "vendor" directory and all its subdirectories
        if should_ignore(rel_root, ignore_patterns):
            continue
            
        # Check each file in this directory
        for filename in filenames:
            # Build relative path for ignore pattern checking
            rel_path = os.path.join(rel_root, filename)
            
            # Skip if the file should be ignored
            # Example: skip files in vendor subdirectories
            if should_ignore(rel_path, ignore_patterns):
                continue
                
            # Check if the filename matches any of our regex patterns
            # Example: "*.py" pattern matches "script.py"
            for pattern in patterns:
                if re.match(pattern, filename):
                    # Add the absolute path to our matches
                    matches.append(os.path.join(root, filename))
                    break  # No need to check other patterns once we have a match
                    
    return matches

def find_existing_license(content: str) -> Tuple[int, int]:
    """
    Find the start and end positions of an existing license header in file content.
    
    Args:
        content: The full content of the file as a string
        
    Returns:
        Tuple of (start_line, end_line) where:
        - start_line: Line number where license starts (0-based), -1 if not found
        - end_line: Line number after license ends (0-based), -1 if not found
        
    This function detects SPDX license headers and standard Apache 2.0 license blocks.
    """
    lines = content.split('\n')
    start_line = -1
    end_line = -1
    in_license_block = False
    
    # Scan through each line looking for license markers
    for i, line in enumerate(lines):
        stripped_line = line.strip()
        
        # Skip shebang lines (#!/usr/bin/env python3, #!/bin/bash, etc.)
        # These should be preserved at the beginning of files
        if stripped_line.startswith('#!/'):
            continue
            
        # Look for the start of a license block
        if not in_license_block:
            # Check for SPDX headers or copyright notices
            if ('SPDX-FileCopyrightText' in stripped_line or 
                'SPDX-License-Identifier' in stripped_line or
                'Copyright (c) NVIDIA CORPORATION' in stripped_line or
                # Go files with block comment starting license
                (stripped_line == '/*' and i + 1 < len(lines) and 
                 ('SPDX-FileCopyrightText' in lines[i + 1] or 
                  'SPDX-License-Identifier' in lines[i + 1] or
                  'Copyright (c) NVIDIA CORPORATION' in lines[i + 1]))):
                start_line = i
                in_license_block = True
        
        # Look for the end of a license block
        if in_license_block and 'limitations under the License' in stripped_line:
            end_line = i + 1
            
            # Include any trailing empty comment lines
            for j in range(i + 1, len(lines)):
                next_line = lines[j].strip()
                if (next_line == '' or  # Empty lines
                    next_line.startswith('#') and next_line.replace('#', '').strip() == '' or
                    next_line == '# ' or
                    next_line == '*/' or  # Go comment block endings
                    next_line.startswith('*') and next_line.replace('*', '').strip() == ''):
                    end_line = j + 1
                else:
                    # Stop when we hit actual code/content
                    break
            break  # Found the end, stop scanning
    
    return start_line, end_line

def normalize_license_for_comparison(license_text: str) -> str:
    """
    Normalize a license text for comparison by removing extra whitespace.
    
    Args:
        license_text: The license text to normalize
        
    Returns:
        Normalized license text with consistent whitespace
        
    This function helps compare existing licenses with generated ones by:
    1. Removing trailing whitespace from each line
    2. Removing leading/trailing empty lines
    3. Preserving the overall structure and content
    
    This ensures that minor whitespace differences don't trigger unnecessary updates.
    """
    lines = license_text.split('\n')
    normalized_lines = []
    
    for line in lines:
        # Keep the line structure but remove trailing whitespace
        # This handles differences like "# " vs "#" at line ends
        normalized_lines.append(line.rstrip())
    
    # Join lines and remove any leading/trailing whitespace from the whole block
    return '\n'.join(normalized_lines).strip()

def insert_license(file_path: str, formatted_license: str, verbose: bool = False) -> None:
    """
    Insert or update the license header in a source file.
    
    Args:
        file_path: Path to the file to modify
        formatted_license: The properly formatted license text to insert
        verbose: Whether to print detailed status messages
        
    This function handles the complete workflow of license management:
    1. Read the existing file content
    2. Check if a license already exists
    3. Compare existing license with target license
    4. Update only if necessary
    5. Preserve important file elements (shebang lines)
    6. Write the updated content back to file
    """
    # Read the current file content
    with open(file_path, 'r') as f:
        content = f.read()
    
    # Look for any existing license header in the file
    start_line, end_line = find_existing_license(content)
    
    lines = content.split('\n')
    
    # Handle existing license
    if start_line != -1 and end_line != -1:
        # Extract the existing license text for comparison
        existing_license = "\n".join(lines[start_line:end_line])
        
        # Normalize both licenses for accurate comparison
        # This prevents updates due to minor whitespace differences
        existing_normalized = normalize_license_for_comparison(existing_license)
        formatted_normalized = normalize_license_for_comparison(formatted_license)
        
        # Check if the license is already correct
        if existing_normalized == formatted_normalized:
            if verbose:
                print(f"License is already formatted in {file_path}")
            return  # No changes needed

        # License exists but needs updating
        print(f"Replacing existing license in {file_path}")
        # Remove the old license by excluding those lines
        lines = lines[:start_line] + lines[end_line:]
        content = '\n'.join(lines)
    else:
        # No existing license found
        print(f"Adding license to {file_path}")
    
    # Clean up the content (remove leading/trailing whitespace)
    content = content.strip()
    
    # Handle special cases for file types that need careful formatting
    if file_path.endswith(('.py', '.sh')):
        # Python and Shell files: preserve shebang lines
        lines = content.split('\n')
        if lines and lines[0].startswith('#!'):
            # Shebang exists: put it first, then license, then content
            # Format: #!/usr/bin/env python3\n\n<license>\n\n<content>
            content = lines[0] + '\n\n' + formatted_license + '\n\n' + '\n'.join(lines[1:])
        else:
            # No shebang: license first, then content
            content = formatted_license + '\n\n' + content
    else:
        # Other file types: license first, then content
        content = formatted_license + '\n\n' + content
    
    # Ensure consistent file ending (exactly one newline)
    content = content.rstrip('\n') + '\n'
    
    # Write the updated content back to the file
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
    5. Include SPDX headers at the beginning of each license block

    Supported file types:
    - Python (.py)       : Uses # comments
    - Shell (.sh)        : Uses # comments
    - Go (.go)           : Uses /* */ block comments
    - YAML (.yml/.yaml)  : Uses # comments
    - Dockerfile         : Uses # comments (includes both "Dockerfile" and files ending in ".Dockerfile")

    Usage:
        ./format_license.py [--license-file PATH] [--root-dir PATH] [--year YEAR] [--verbose]

    Arguments:
        --license-file : Path to the Apache 2.0 license file (default: LICENSE)
        --root-dir     : Root directory to search for files (default: current directory)
        --year         : Year to use in SPDX copyright header (default: current year)
        --verbose      : Show detailed messages, including when licenses are already formatted

    Example:
        # Format all supported files in the current directory
        ./format_license.py

        # Format files using a specific license file and directory with verbose output
        ./format_license.py --license-file /path/to/LICENSE --root-dir /path/to/project --verbose

        # Format files with a specific year in the SPDX header
        ./format_license.py --year 2024

    Note:
        The script automatically ignores common vendor directories.
        The chart/ directory is also ignored by default. See BUILT_IN_IGNORE_PATTERNS for more details.
    """
    parser = argparse.ArgumentParser(description='Format and apply license headers to source files')
    parser.add_argument('--license-file', default='LICENSE',  help='Path to the license template file')
    parser.add_argument('--root-dir', default='.',  help='Root directory to search for files')
    parser.add_argument('--year', help='Year to use in SPDX copyright header (default: current year)')
    parser.add_argument('--verbose', action='store_true', help='Show detailed messages, including when licenses are already formatted')
    args = parser.parse_args()

    # Step 1: Read and extract the license boilerplate from the LICENSE file
    # This automatically extracts just the part needed for source file headers
    license_text = read_license_template(args.license_file)

    # Step 2: Get changed files from git
    changed_files = get_changed_files_from_git(args.root_dir)

    if not changed_files:
        print("No changed files found.")
        return

    if args.verbose:
        print(f"Found {len(changed_files)} changed file(s)")

    # Step 3: Set up file/directory filtering
    # Use built-in patterns to ignore vendor directories, etc.
    ignore_patterns = BUILT_IN_IGNORE_PATTERNS

    # Step 4: Process each changed file
    for file_path in changed_files:
        # Skip deleted files (they appear in git diff but don't exist on disk)
        if not os.path.exists(file_path):
            continue
            
        # Skip ignored files
        rel_path = os.path.relpath(file_path, args.root_dir)
        if should_ignore(rel_path, ignore_patterns):
            continue

        # Check if file matches any supported pattern
        filename = os.path.basename(file_path)
        for pattern, comment_prefix in COMMENT_STYLES.items():
            if re.match(pattern, filename):
                # Format license for this file type
                formatted_license = format_license(license_text, comment_prefix, args.year)
                # Process the file
                insert_license(file_path, formatted_license, args.verbose)
                break

if __name__ == '__main__':
    main()
