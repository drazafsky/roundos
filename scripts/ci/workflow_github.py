"""Small GitHub CLI wrapper for the repository workflow tools."""

import json
import re
import subprocess

REPO = 'drazafsky/roundos'


class ToolError(ValueError):
    pass


def gh(*args):
    result = subprocess.run(['gh', *args], capture_output=True, text=True, timeout=45)
    if result.returncode:
        detail = result.stderr.strip().splitlines()
        raise ToolError(detail[0] if detail else f'GitHub CLI exited with code {result.returncode}')
    return result.stdout


class GitHub:
    def __init__(self, repo=REPO):
        if not re.fullmatch(r'[\w.-]+/[\w.-]+', repo, re.ASCII):
            raise ToolError('Expected a repository in owner/name form')
        self.repo = repo

    def api(self, path, *args, missing_ok=False):
        try:
            return json.loads(gh('api', f'repos/{self.repo}/{path}', *args) or 'null')
        except ToolError as error:
            if missing_ok and '(HTTP 404)' in str(error):
                return None
            raise

    def pages(self, path, key=None):
        separator = '&' if '?' in path else '?'
        for page in range(1, 31):
            response = self.api(f'{path}{separator}per_page=100&page={page}')
            batch = response[key] if key else response
            yield from batch
            if len(batch) < 100:
                return
        raise ToolError('Too many GitHub results to verify completely')

    def graphql(self, query, **variables):
        args = [arg for name, val in variables.items() for arg in ('-F' if type(val) is int else '-f', f'{name}={val}')]
        result = json.loads(gh('api', 'graphql', '-f', f'query={query}', *args))
        if result.get('errors'):
            raise ToolError(result['errors'][0]['message'])
        return result['data']

    def repo_graphql(self, query, **variables):
        owner, name = self.repo.split('/')
        return self.graphql(query, owner=owner, name=name, **variables)['repository']


def positive(value):
    number = int(value)
    if number <= 0:
        raise ValueError('Expected a positive number')
    return number
