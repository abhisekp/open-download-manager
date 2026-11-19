"""Validate the SBOM against the pinned official CycloneDX schema and references."""
import argparse
import json
import pathlib

import jsonschema
from referencing import Registry, Resource


def validate(path):
    folder = pathlib.Path(__file__).with_name('schemas')
    resources = []
    for file in folder.glob('*.schema.json'):
        schema = json.loads(file.read_text(encoding='utf-8'))
        resource = Resource.from_contents(schema)
        resources.append((schema['$id'], resource))
        resources.append(('http://cyclonedx.org/schema/' + file.name, resource))
        resources.append(('https://cyclonedx.org/schema/' + file.name, resource))
    registry = Registry().with_resources(resources)
    schema = json.loads((folder / 'bom-1.6.schema.json').read_text())
    bom = json.loads(path.read_text(encoding='utf-8'))
    jsonschema.Draft7Validator(schema, registry=registry,
                              format_checker=jsonschema.FormatChecker()).validate(bom)
    refs = {bom['metadata']['component']['bom-ref']}
    for component in bom['components']:
        if component['bom-ref'] in refs:
            raise ValueError('Duplicate component reference')
        refs.add(component['bom-ref'])
    for dependency in bom.get('dependencies', []):
        if dependency['ref'] not in refs or any(ref not in refs for ref in dependency.get('dependsOn', [])):
            raise ValueError('Unresolved component reference')
    print('PASS: official CycloneDX 1.6 schema and component references')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('path', type=pathlib.Path)
    validate(parser.parse_args().path)
