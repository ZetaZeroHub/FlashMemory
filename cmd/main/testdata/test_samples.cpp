// C++ test file for parser testing

#include <iostream>
#include <string>

// Simple function
void simpleFunction() {
    std::cout << "Hello, world!" << std::endl;
}

// Function with parameters
std::string functionWithParams(const std::string& name, int age) {
    return "Hello, " + name + "! You are " + std::to_string(age) + " years old.";
}

// Class with methods
class ClassWithMethods {
private:
    std::string name;
    int age;
    
    // Private helper method
    std::string privateHelper() {
        std::string result = name;
        for (auto& c : result) {
            c = std::toupper(c);
        }
        return result;
    }
    
public:
    ClassWithMethods(const std::string& name, int age) : name(name), age(age) {}
    
    std::string getInfo() {
        return name + " is " + std::to_string(age) + " years old";
    }
    
    void updateAge(int newAge) {
        age = newAge;
        simpleFunction();
        functionWithParams(name, age);
        privateHelper();
    }
};

// Main function
int main() {
    ClassWithMethods person("Alice", 30);
    std::cout << person.getInfo() << std::endl;
    person.updateAge(31);
    return 0;
}