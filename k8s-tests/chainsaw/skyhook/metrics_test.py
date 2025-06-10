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

import argparse
import os
import sys
import time
import re
import urllib.request
import urllib.error

# Helper to parse Prometheus metrics lines
# Example: metric_name{key1="val1",key2="val2"} 123
METRIC_LINE_RE = re.compile(r'^(?P<name>[a-zA-Z_:][a-zA-Z0-9_:]*)\{(?P<labels>[^}]*)\}\s+(?P<value>.+)$')

def parse_labels(labels_str):
    labels = {}
    if not labels_str:
        return labels
    for pair in labels_str.split(','):
        k, v = pair.split('=', 1)
        labels[k.strip()] = v.strip().strip('"')
    return labels


def metric_matches(line, metric_name, tags, metric_value):
    m = METRIC_LINE_RE.match(line)
    if not m:
        return False
    if m.group('name') != metric_name:
        return False
    labels = parse_labels(m.group('labels'))
    for k, v in tags.items():
        if labels.get(k) != v:
            return False
    # Compare value as string (exact match)
    return m.group('value') == metric_value


def main():
    parser = argparse.ArgumentParser(description="Check for a Prometheus metric with specific tags and value.")
    parser.add_argument('metric_name', help='Name of the metric to search for')
    parser.add_argument('metric_value', help='Value the metric should have (exact match)')
    parser.add_argument('-t', '--tag', action='append', default=[], help='Tag filter in key=value format (can be used multiple times)')
    parser.add_argument('--url', default='http://127.0.0.1:8080/metrics', help='Metrics endpoint URL')
    parser.add_argument('--not-found', action='store_true', help='Succeed if the metric is NOT found (invert match logic)')
    args = parser.parse_args()

    # Parse tags
    tags = {}
    for tag in args.tag:
        if '=' not in tag:
            print(f"Invalid tag format: {tag}. Use key=value.", file=sys.stderr)
            sys.exit(2)
        k, v = tag.split('=', 1)
        tags[k] = v

    TIMEOUT = float(os.environ.get('TIMEOUT', 30))
    PERIOD = float(os.environ.get('PERIOD', 0.1))

    start_time = time.time()
    end_time = start_time + TIMEOUT

    mode = 'url'
    if args.url == '-':
        mode = 'stdin'

    while True:
        if mode == 'stdin':
            lines = sys.stdin.readlines()
        else:
            try:
                with urllib.request.urlopen(args.url) as resp:
                    content = resp.read().decode('utf-8')
                    lines = content.splitlines()
            except Exception as e:
                print(f"Error fetching metrics: {e}", file=sys.stderr)
                lines = []
        found = False
        for line in lines:
            if metric_matches(line, args.metric_name, tags, args.metric_value):
                if mode == 'stdin':
                    print(line)
                found = True
                break

        success = (found and not args.not_found) or (not found and args.not_found)
        if success:
            sys.exit(0)

        msg = f"Metric {args.metric_name} with tags {tags} and value {args.metric_value}"
        if args.not_found:
            msg += " was found (should be absent)"
        else:
            msg += " was not found"
        if mode == 'stdin':
            print(msg, file=sys.stderr)
            sys.exit(1)
       
        if time.time() >= end_time:
            msg += f" after {TIMEOUT} seconds"
            print(msg, file=sys.stderr)
            sys.exit(1)
        time.sleep(PERIOD)

if __name__ == '__main__':
    main()
