#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0
from enum import Enum

class SortableEnum(Enum):

    LATEST: str

    def __str__(self):
        return self.value

    def __eq__(self, other):
        if isinstance(other, SortableEnum):
            return self.value == other.value
        elif isinstance(other, str):
            return self.value == other
        return False

    def __ne__(self, other):
        return not self.__eq__(other)

    def __lt__(self, other):
        if self.__eq__(other):
            return False
        if isinstance(other, SortableEnum):
            if other == self.LATEST:
                return True
            if self == self.LATEST:
                return False
            return int(self.value.strip('v')) < int(other.value.strip('v'))
        elif isinstance(other, str):
            if other == self.LATEST.value:
                return True
            if self == self.LATEST:
                return False
            return int(self.value.strip('v')) < int(other.lower().strip('v'))
        return NotImplemented

    def __le__(self, other):
        return self.__eq__(other) or self.__lt__(other)

    def __gt__(self, other):
        if self.__eq__(other):
            return False
        return not self.__le__(other)

    def __ge__(self, other):
        return self.__eq__(other) or not self.__lt__(other)
    
class SchemaVersion(SortableEnum):
    V1 = "v1"
    LATEST = "latest"
    

# schema is passable here to make it testable. Not expected to be used in production
def get_latest_schema(schema=SchemaVersion) -> SchemaVersion:
    key_to_value = [(k, int(v.value.lower().replace('v',''))) for k,v in schema._member_map_.items() if v != 'latest']
    latest_key, _ = sorted(key_to_value, key=lambda x: x[1])[-1]
    return schema[latest_key]
