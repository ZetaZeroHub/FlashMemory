# Python test file for parser testing

import os
import sys
from datetime import datetime

# Simple function
def simple_function():
    print("Hello, world!")

# Function with parameters
def function_with_params(name, age):
    return f"Hello, {name}! You are {age} years old."

# Class with methods
class ClassWithMethods:
    def __init__(self, name, age):
        self.name = name
        self.age = age
    
    def get_info(self):
        return f"{self.name} is {self.age} years old"
    
    def update_age(self, new_age):
        self.age = new_age
        simple_function()
        function_with_params(self.name, self.age)
        self._private_helper()
    
    def _private_helper(self):
        return self.name.upper()

# Main execution
if __name__ == "__main__":
    person = ClassWithMethods("Alice", 30)
    print(person.get_info())
    person.update_age(31)