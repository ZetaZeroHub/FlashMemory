// JavaScript test file for parser testing

// Simple function
function simpleFunction() {
    console.log("Hello, world!");
}

// Function with parameters
function functionWithParams(name, age) {
    return `Hello, ${name}! You are ${age} years old.`;
}

// Class with methods
class ClassWithMethods {
    constructor(name, age) {
        this.name = name;
        this.age = age;
    }
    
    getInfo() {
        return `${this.name} is ${this.age} years old`;
    }
    
    updateAge(newAge) {
        this.age = newAge;
        simpleFunction();
        functionWithParams(this.name, this.age);
        this._privateHelper();
    }
    
    _privateHelper() {
        return this.name.toUpperCase();
    }
}

// Arrow function
const arrowFunction = (x) => {
    return x * x;
};

// Function expression
const functionExpression = function(a, b) {
    return a + b;
};

// Main execution
const person = new ClassWithMethods("Alice", 30);
console.log(person.getInfo());
person.updateAge(31);