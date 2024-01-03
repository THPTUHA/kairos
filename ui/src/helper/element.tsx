export function hasSpecialCharacter(text: string) {
    const regex = /[!@#$%^&*()+{}\[\]:;<>,.?~\\/-]/;
    return regex.test(text);
  }

  export function hasSpecialCharacter2(text: string) {
    const regex = /[!@#$%^&*()+{}\[\]:;<>,?~\\/-]/;
    return regex.test(text);
  }

export function ColorElement(e: string, dest: boolean){
    if(dest){
        return <span className="text-green-500">{e}</span>
    }
    if(e.startsWith(".")){
        return <span className="text-yellow-500">{e}</span>
    }
    
    if(e.startsWith("(")){
        return <span className="text-orange-500">{e}</span>
    }

    if(!hasSpecialCharacter(e) && e.trim()){
        return <span className="text-blue-500">{e}</span>
    }
    return <span >{e}</span>
}

const symbols = ["_ping"]

export function colorElement(e: string, dest: boolean){
    if(dest && !e.startsWith(".") && !symbols.includes(e)){
        return "text-green-500"
    }
    if(e.startsWith(".")){
        return "text-blue-500"
    }
    
    if(e.startsWith("(") || symbols.includes(e)){
        return "text-amber-500"
    }
    if(e.includes("{") || e.includes("}")){
        return ""
    }

    if(!hasSpecialCharacter(e) && e.trim()){
        return "text-violet-500"
    }
    return "text-red-500"
}